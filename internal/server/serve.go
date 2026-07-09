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
		if matched.Response.DelayDuration > 0 {
			time.Sleep(matched.Response.DelayDuration)
		}
		status, respBody := s.writeResponse(w, &matched.Response, r, body)
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
func (s *Server) writeResponse(w http.ResponseWriter, resp *rule.Response, r *http.Request, reqBody []byte) (int, string) {
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
	for k, v := range resp.Headers {
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
		out, err := rule.ExecuteTemplate(body, r, reqBody, func(f *rule.RequestFilter) int64 {
			n := int64(s.journal.Count(f))
			if currentRequestMatches(f, r) {
				n++ // include the in-flight request; it's journaled only after this write
			}
			return n
		})
		if err != nil {
			s.logf("template error: %v", err)
			return resp.Status, ""
		}
		body = out
	}
	fmt.Fprint(w, body)
	return resp.Status, body
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
