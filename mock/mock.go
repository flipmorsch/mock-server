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
	"fmt"
	"net"
	"net/http"
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

// Request is a request the mock received.
type Request struct {
	Method  string
	Path    string
	Query   string
	Headers map[string]string // sensitive headers (Authorization, Cookie, …) are redacted
	Body    string
	Status  int // status the mock responded with
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

// Reset clears the record of received requests (call between test cases).
func (s *Server) Reset() { s.journal.Clear() }

// Received returns the requests the mock has handled, oldest first (capped at the
// journal's retained window; use Count/Verify for exact tallies).
func (s *Server) Received() []Request {
	entries := s.journal.Entries(nil)
	out := make([]Request, len(entries))
	for i, e := range entries {
		out[i] = Request{
			Method:  e.Method,
			Path:    e.Path,
			Query:   e.Query,
			Headers: e.Headers,
			Body:    e.Body,
			Status:  e.Status,
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
			n, orAny(method), orAny(path), got, s.summary())
	}
	return nil
}

// VerifyCalled asserts the mock received at least one request matching method and
// path (either may be empty for "any").
func (s *Server) VerifyCalled(method, path string) error {
	if s.Count(method, path) == 0 {
		return fmt.Errorf("expected at least one request matching %s %s, got none\nreceived:\n%s",
			orAny(method), orAny(path), s.summary())
	}
	return nil
}

func (s *Server) summary() string {
	entries := s.journal.Entries(nil)
	if len(entries) == 0 {
		return "  (no requests received)"
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "  %s %s → %d\n", e.Method, e.Path, e.Status)
	}
	return strings.TrimRight(b.String(), "\n")
}

func orAny(s string) string {
	if s == "" {
		return "<any>"
	}
	return s
}
