package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
		entry.Duration = time.Since(start)
		s.logf("%s %s → %d (matched: %s)", r.Method, r.URL.Path, matched.Response.Status, matched.Name)
		entry.Matched = matched.Name
		entry.MatchedID = matched.ID
		entry.Status = matched.Response.Status
		s.journal.Record(entry)
		s.writeResponse(w, &matched.Response, r, body)
		return
	}

	entry.Duration = time.Since(start)
	s.logf("%s %s → 404 (no match)", r.Method, r.URL.Path)
	entry.Status = 404
	s.journal.Record(entry)
	http.NotFound(w, r)
}

func (s *Server) writeResponse(w http.ResponseWriter, resp *rule.Response, r *http.Request, reqBody []byte) {
	body := resp.Body
	if resp.BodyFile != "" {
		data, err := os.ReadFile(resp.BodyFile)
		if err != nil {
			s.logf("body_file error: %v", err)
			http.Error(w, "body_file read failed", http.StatusInternalServerError)
			return
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
		return
	}
	if resp.Template {
		var err error
		body, err = rule.ExecuteTemplate(body, r, reqBody, func(f *rule.RequestFilter) int64 { return int64(s.journal.Count(f)) })
		if err != nil {
			s.logf("template error: %v", err)
			return
		}
	}
	fmt.Fprint(w, body)
}
