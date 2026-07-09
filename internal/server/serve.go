package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/flipmorsch/mock-server/internal/rule"
)

// SetLogger sets the per-request logger. When nil (the default), serving is
// silent — which is what an embedded/library caller wants. The CLI sets it so
// each request is logged.
func (s *Server) SetLogger(l *log.Logger) {
	s.logger = l
}

func (s *Server) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

// ServeMock matches the request against the rule set, writes the mock response,
// and records the exchange in the journal. It handles mock traffic only; callers
// route /_ui/ and /__admin/ before delegating here.
func (s *Server) ServeMock(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	reqHeaders := make(map[string]string)
	for k := range r.Header {
		reqHeaders[http.CanonicalHeaderKey(k)] = r.Header.Get(k)
	}

	entry := JournalEntry{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.RawQuery,
		Headers: reqHeaders,
		Body:    string(body),
	}

	matched, misses := s.MatchRule(r, body)
	entry.Explanations = misses
	if matched != nil {
		// Select the response before sleeping so sequence order follows
		// arrival, not who finishes a per-element delay first (ADR-0007).
		// Trade-off: the position advances here, so a later writeResponse
		// failure (e.g. a body_file deleted after the load-time readability
		// check) still consumes this element. Accepted — advancing at match
		// time is what makes concurrent ordering deterministic.
		resp := &matched.Response
		if matched.Sequenced() {
			i := s.seq.selectIndex(matched.ID, len(matched.Responses))
			resp = &matched.Responses[i]
			// Record which element served this request, so the journal can
			// answer "why this response on this call?" (i is clamped to the
			// last element once the sequence is exhausted — last sticks).
			entry.SeqPos = i + 1
			entry.SeqTotal = len(matched.Responses)
		}
		if resp.DelayDuration > 0 {
			time.Sleep(resp.DelayDuration)
		}
		params := rule.PathParams(matched.Request.PathMode, matched.Request.Path, r.URL.Path)
		status, respBody := s.writeResponse(w, resp, r, body, params)
		entry.Duration = time.Since(start)
		s.logf("%s %s → %d (matched: %s)", r.Method, r.URL.Path, status, matched.Name)
		entry.Matched = matched.Name
		entry.MatchedID = matched.ID
		entry.Status = status
		entry.ResponseBody = respBody
		s.journal.Record(entry)
		return
	}

	entry.Duration = time.Since(start)
	s.logf("%s %s → 404 (no match)", r.Method, r.URL.Path)
	entry.Status = 404
	s.journal.Record(entry)
	http.NotFound(w, r)
}

// writeResponse writes the mock response and returns the status and body it
// actually produced, so the caller can record them in the journal.
func (s *Server) writeResponse(w http.ResponseWriter, resp *rule.Response, r *http.Request, reqBody []byte, params map[string]string) (int, string) {
	body := resp.Body
	if resp.BodyFile != "" {
		data, err := os.ReadFile(resp.BodyFile)
		if err != nil {
			s.logf("body_file error: %v", err)
			const msg = "body_file read failed"
			http.Error(w, msg, http.StatusInternalServerError)
			return http.StatusInternalServerError, msg
		}
		body = string(data)
	}
	counter := s.templateCounter(r)
	for k, v := range resp.Headers {
		if resp.Template {
			// Header values are templated too (keys stay literal), so a 201 can
			// carry Location: /users/{{.Param "id"}}. A template syntax error
			// falls back to the raw value rather than failing the response.
			if out, err := rule.ExecuteTemplate(v, r, reqBody, params, counter); err == nil {
				v = out
			} else {
				s.logf("template error (header %s): %v", k, err)
			}
		}
		w.Header().Set(k, v)
	}
	if _, ok := resp.Headers["Content-Type"]; !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(resp.Status)
	if body == "" {
		return resp.Status, ""
	}
	if resp.Template {
		out, err := rule.ExecuteTemplate(body, r, reqBody, params, counter)
		if err != nil {
			s.logf("template error: %v", err)
			return resp.Status, ""
		}
		body = out
	}
	fmt.Fprint(w, body)
	return resp.Status, body
}

// templateCounter builds the requestCount backing func for one request: the
// journal tally plus the in-flight request, which isn't recorded until after the
// response is written. Shared by header and body templating.
func (s *Server) templateCounter(r *http.Request) func(*rule.RequestFilter) int64 {
	return func(f *rule.RequestFilter) int64 {
		n := int64(s.journal.Count(f))
		if currentRequestMatches(f, r) {
			n++
		}
		return n
	}
}

// currentRequestMatches reports whether the in-flight request satisfies the
// filter's method/path — the only dimensions requestCount sets. It lets template
// counts include the current request, which isn't journaled until after the
// response is written.
func currentRequestMatches(f *rule.RequestFilter, r *http.Request) bool {
	if f.Method != "" && !strings.EqualFold(f.Method, r.Method) {
		return false
	}
	if f.Path != "" && !rule.PathMatches(f.PathMode, f.Path, r.URL.Path) {
		return false
	}
	return true
}
