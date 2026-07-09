// Package mock runs mock-server in-process for use as a test double in Go tests.
//
//	m, err := mock.Start(`
//	rules:
//	  - request: {method: GET, path: /users/1}
//	    response: {status: 200, body: '{"id":1}'}
//	`)
//	if err != nil { t.Fatal(err) }
//	defer m.Close()
//
//	resp, _ := http.Get(m.URL() + "/users/1")
//	// ... exercise the code under test ...
//	if err := m.Verify("GET", "/users/1", 1); err != nil { t.Error(err) }
package mock

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/flipmorsch/mock-server/internal/rule"
	"github.com/flipmorsch/mock-server/internal/server"
)

// Server is a running in-process mock. Create one with Start; call Close when done.
type Server struct {
	inner   *server.Server
	journal *server.Journal
	httpSrv *http.Server
	url     string
}

// Request is a request the mock received, plus how the mock responded.
type Request struct {
	Method       string
	Path         string
	Query        string
	Headers      map[string]string // sensitive headers (Authorization, Cookie, …) are redacted
	Body         string
	Status       int    // status the mock responded with
	ResponseBody string // body the mock returned
}

// Match selects received requests. Empty fields match anything.
//
// JSONBody, when set, requires the request body to contain it as a JSON subset
// (object fields partial, arrays element-wise, scalars equal). Query and Headers
// require each given key to be present on the request; a non-empty value must match
// exactly, an empty value asserts presence only.
//
// The sensitive headers redacted in the journal (Authorization, Cookie, X-Api-Key,
// …) are stored as "[REDACTED]", so a header Match on them can only assert presence
// (empty value), never their real value.
type Match struct {
	Method   string
	Path     string
	JSONBody string
	Query    map[string]string
	Headers  map[string]string
}

// Start launches a mock on a random loopback port, serving rules parsed from the
// given YAML (the same schema as the CLI config file). The returned Server is
// ready to receive requests; call Close to shut it down.
func Start(configYAML string) (*Server, error) {
	cfg, err := rule.ParseConfig([]byte(configYAML))
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	journal := server.NewJournal()
	inner := server.NewServer(cfg, "", journal, false)
	httpSrv := &http.Server{Handler: http.HandlerFunc(inner.ServeMock)}
	go httpSrv.Serve(ln)
	return &Server{
		inner:   inner,
		journal: journal,
		httpSrv: httpSrv,
		url:     "http://" + ln.Addr().String(),
	}, nil
}

// URL is the base URL of the running mock (e.g. http://127.0.0.1:54321). Point
// the code under test at this.
func (s *Server) URL() string { return s.url }

// Close shuts the mock down.
func (s *Server) Close() error { return s.httpSrv.Close() }

// Reset clears the record of received requests and rewinds every response
// sequence to its first element — a clean slate for the next test case.
func (s *Server) Reset() { s.inner.Reset() }

// Received returns the requests the mock has handled, oldest first (capped at the
// journal's retained window; use Count/Verify for exact tallies).
func (s *Server) Received() []Request {
	entries := s.journal.Entries(nil)
	out := make([]Request, len(entries))
	for i, e := range entries {
		out[i] = Request{
			Method:       e.Method,
			Path:         e.Path,
			Query:        e.Query,
			Headers:      e.Headers,
			Body:         e.Body,
			Status:       e.Status,
			ResponseBody: e.ResponseBody,
		}
	}
	return out
}

// Count returns how many received requests match method and path; either may be
// empty to match any. Counts are sound regardless of the retained window.
func (s *Server) Count(method, path string) int {
	return s.journal.Count(&rule.RequestFilter{Method: method, Path: path})
}

// Verify asserts the mock received exactly n requests matching method and path
// (either may be empty for "any"). On mismatch the error lists what was actually
// received, so a failed assertion diagnoses itself.
func (s *Server) Verify(method, path string, n int) error {
	if got := s.Count(method, path); got != n {
		return fmt.Errorf("expected %d request(s) matching %s %s, got %d\nreceived:\n%s",
			n, orAny(method), orAny(path), got, s.summary(nil))
	}
	return nil
}

// VerifyCalled asserts the mock received at least one request matching method and
// path (either may be empty for "any").
func (s *Server) VerifyCalled(method, path string) error {
	if s.Count(method, path) == 0 {
		return fmt.Errorf("expected at least one request matching %s %s, got none\nreceived:\n%s",
			orAny(method), orAny(path), s.summary(nil))
	}
	return nil
}

// CountMatch returns how many received requests satisfy m. A method/path-only
// Match uses monotonic tallies and stays sound regardless of the retained window;
// once Query, Headers, or JSONBody is set the count is scoped to that window.
func (s *Server) CountMatch(m Match) int {
	base := &rule.RequestFilter{Method: m.Method, Path: m.Path}
	if m.JSONBody == "" && len(m.Query) == 0 && len(m.Headers) == 0 {
		return s.journal.Count(base)
	}
	n := 0
	for _, e := range s.journal.Entries(base) {
		if m.extraMatch(&e) {
			n++
		}
	}
	return n
}

// extraMatch applies the dimensions the coarse method/path filter can't: JSON body
// subset, query params, and headers.
//
// ponytail: header/query matching lives here, not in rule.requestFilterMatch, so the
// frozen /__admin/ filter semantics stay put — here an empty value means "present
// with any value", which is how a redacted sensitive header is asserted.
func (m Match) extraMatch(e *server.JournalEntry) bool {
	if m.JSONBody != "" && !rule.JSONBodyMatches(m.JSONBody, e.Body) {
		return false
	}
	if len(m.Query) > 0 {
		q, _ := url.ParseQuery(e.Query)
		for k, v := range m.Query {
			vals, ok := q[k]
			if !ok || (v != "" && !slices.Contains(vals, v)) {
				return false
			}
		}
	}
	for k, v := range m.Headers {
		hv, ok := e.Headers[http.CanonicalHeaderKey(k)]
		if !ok || (v != "" && hv != v) {
			return false
		}
	}
	return true
}

// VerifyMatch asserts the mock received exactly n requests satisfying m. On
// mismatch the error lists what was actually received.
func (s *Server) VerifyMatch(m Match, n int) error {
	if got := s.CountMatch(m); got != n {
		return fmt.Errorf("expected %d request(s) matching %s, got %d\nreceived:\n%s",
			n, m, got, s.summary(&m))
	}
	return nil
}

// VerifyAtLeast asserts the mock received at least n requests satisfying m.
func (s *Server) VerifyAtLeast(m Match, n int) error {
	if got := s.CountMatch(m); got < n {
		return fmt.Errorf("expected at least %d request(s) matching %s, got %d\nreceived:\n%s",
			n, m, got, s.summary(&m))
	}
	return nil
}

// VerifyAtMost asserts the mock received at most n requests satisfying m.
func (s *Server) VerifyAtMost(m Match, n int) error {
	if got := s.CountMatch(m); got > n {
		return fmt.Errorf("expected at most %d request(s) matching %s, got %d\nreceived:\n%s",
			n, m, got, s.summary(&m))
	}
	return nil
}

func (m Match) String() string {
	s := orAny(m.Method) + " " + orAny(m.Path)
	if len(m.Query) > 0 {
		s += " ?" + kvString(m.Query)
	}
	if len(m.Headers) > 0 {
		s += " H{" + kvString(m.Headers) + "}"
	}
	if m.JSONBody != "" {
		s += " body⊇" + m.JSONBody
	}
	return s
}

// kvString renders a filter map deterministically: "k=v" pairs, or bare "k" for a
// presence-only (empty value) entry, sorted by key.
func kvString(kv map[string]string) string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		if kv[k] == "" {
			parts[i] = k
		} else {
			parts[i] = k + "=" + kv[k]
		}
	}
	return strings.Join(parts, ",")
}

// summary renders the received requests for a failed assertion. Each line carries
// the request's (truncated) body — the thing a body assertion exists to explain. If
// m carries a JSONBody, every method/path-matching request is annotated with the
// first JSON path that differed, so the failure diagnoses itself.
func (s *Server) summary(m *Match) string {
	entries := s.journal.Entries(nil)
	if len(entries) == 0 {
		return "  (no requests received)"
	}
	var b strings.Builder
	for i := range entries {
		e := &entries[i]
		path := e.Path
		if e.Query != "" {
			path += "?" + e.Query
		}
		fmt.Fprintf(&b, "  %s %s → %d\n", e.Method, path, e.Status)
		if e.Body != "" {
			fmt.Fprintf(&b, "    body: %s\n", truncBody(e.Body))
		}
		if m != nil && m.JSONBody != "" && methodPathMatch(m, e) {
			if p, wv, gv, ok := rule.JSONBodyDiff(m.JSONBody, e.Body); !ok {
				label := "JSONBody"
				if p != "" {
					label += "." + p
				}
				fmt.Fprintf(&b, "    ↳ %s: got %s, want %s\n", label, jsonVal(gv), jsonVal(wv))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func methodPathMatch(m *Match, e *server.JournalEntry) bool {
	if m.Method != "" && !strings.EqualFold(m.Method, e.Method) {
		return false
	}
	return m.Path == "" || m.Path == e.Path
}

func truncBody(b string) string {
	const max = 256
	if len(b) > max {
		return b[:max] + "…"
	}
	return b
}

func jsonVal(v any) string {
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

func orAny(s string) string {
	if s == "" {
		return "<any>"
	}
	return s
}
