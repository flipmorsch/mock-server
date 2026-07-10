package server

import (
	"time"
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/flipmorsch/mock-server/internal/rule"
)

// Hop-by-hop headers that must not be forwarded or captured (RFC 9110 §7.6.1).
var hopByHop = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"TE":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// Headers redacted in the journal (v1.0.1); same set stripped from captured responses.
var redacted = map[string]bool{
	"Authorization":     true,
	"Proxy-Authorization": true,
	"Cookie":            true,
	"Set-Cookie":        true,
	"X-Api-Key":         true,
	"X-Auth-Token":      true,
}

func (s *Server) recordAndProxy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqBody, _ := io.ReadAll(r.Body)
	r.Body.Close()

	upstreamReq, err := s.buildUpstreamRequest(r, reqBody)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}

	resp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	genRule := s.generateRule(r, resp, respBody)
	if err := s.appendCapturedRule(genRule); err != nil {
		s.logf("recording: write error: %v", err)
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	entry := JournalEntry{
		Method:       r.Method,
		Path:         r.URL.Path,
		Query:        r.URL.RawQuery,
		Body:         string(reqBody),
		Status:       resp.StatusCode,
		ResponseBody: string(respBody),
		Duration:     time.Since(start),
		Matched:      fmt.Sprintf("recorded: %s %s → %d", r.Method, r.URL.Path, resp.StatusCode),
	}
	s.journal.Record(entry)
	s.logf("%s %s → %d (recorded)", r.Method, r.URL.Path, resp.StatusCode)
}

func (s *Server) buildUpstreamRequest(r *http.Request, body []byte) (*http.Request, error) {
	u := strings.TrimRight(s.recordCfg.Upstream, "/") + r.URL.Path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, vs := range r.Header {
		if hopByHop[k] {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return req, nil
}

func (s *Server) generateRule(r *http.Request, resp *http.Response, body []byte) rule.Rule {
	ct := resp.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	capturedBody := string(body)
	if !isTextContent(mediaType) {
		capturedBody = fmt.Sprintf("[binary, %d bytes]", len(body))
	}

	respHeaders := make(map[string]string)
	for k, vs := range resp.Header {
		cKey := http.CanonicalHeaderKey(k)
		if hopByHop[cKey] || redacted[cKey] {
			continue
		}
		respHeaders[cKey] = vs[0]
	}

	return rule.Rule{
		Name: r.Method + " " + r.URL.Path,
		Request: rule.Request{
			Method: r.Method,
			Path:   r.URL.Path,
		},
		Response: rule.Response{
			Status:  resp.StatusCode,
			Headers: respHeaders,
			Body:    capturedBody,
		},
	}
}

func isTextContent(mediaType string) bool {
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/x-www-form-urlencoded":
		return true
	}
	return false
}

func (s *Server) appendCapturedRule(r rule.Rule) error {
	s.mu.Lock()
	s.recordRules = append(s.recordRules, r)
	rules := make([]rule.Rule, len(s.recordRules))
	copy(rules, s.recordRules)
	s.mu.Unlock()

	if s.recordCfg.OutputPath == "" {
		return writeRuleStdout(r)
	}
	return writeConfigAtomic(s.recordCfg.OutputPath, rules)
}

func writeRuleStdout(r rule.Rule) error {
	b, err := yaml.Marshal(struct {
		Rules []rule.Rule `yaml:"rules"`
	}{Rules: []rule.Rule{r}})
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}

func writeConfigAtomic(path string, rules []rule.Rule) error {
	cfg := struct {
		Rules []rule.Rule `yaml:"rules"`
	}{Rules: rules}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

