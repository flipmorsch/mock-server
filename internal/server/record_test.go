package server

import (
	"gopkg.in/yaml.v3"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flipmorsch/mock-server/internal/rule"
)

func TestRecordAndProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "kept")
		w.Header().Set("Set-Cookie", "secret=redacted")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"echo": %q}`, string(body))
	}))
	defer upstream.Close()

	tmp := filepath.Join(t.TempDir(), "recorded.yaml")
	srv := newTestServer(t)
	srv.SetRecordConfig(RecordConfig{
		Upstream:   upstream.URL,
		OutputPath: tmp,
	})

	req := httptest.NewRequest("POST", "/api/items", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	srv.ServeMock(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	var resp struct{ Echo string }
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Echo != `{"name":"test"}` {
		t.Fatalf("echo = %q, want {\"name\":\"test\"}", resp.Echo)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read recorded file: %v", err)
	}
	yamlStr := string(data)

	if !strings.Contains(yamlStr, "method: POST") {
		t.Errorf("recorded rule missing method: POST:\n%s", yamlStr)
	}
	if !strings.Contains(yamlStr, "path: /api/items") {
		t.Errorf("recorded rule missing path: /api/items:\n%s", yamlStr)
	}
	if !strings.Contains(yamlStr, "status: 201") {
		t.Errorf("recorded rule missing status: 201:\n%s", yamlStr)
	}
	if !strings.Contains(yamlStr, "X-Custom: kept") {
		t.Errorf("recorded rule missing X-Custom header:\n%s", yamlStr)
	}
	if strings.Contains(yamlStr, "Set-Cookie") {
		t.Errorf("recorded rule should NOT contain redacted Set-Cookie:\n%s", yamlStr)
	}
	if strings.Contains(yamlStr, "Connection") {
		t.Errorf("recorded rule should NOT contain hop-by-hop Connection:\n%s", yamlStr)
	}

	var cfg rule.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("recorded YAML invalid: %v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}
	if cfg.Rules[0].Response.Body != `{"echo": "{\"name\":\"test\"}"}` {
		t.Errorf("body = %q", cfg.Rules[0].Response.Body)
	}
}

func TestRecordAndProxyMultipleRequests(t *testing.T) {
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "response %d", callCount)
	}))
	defer upstream.Close()

	tmp := filepath.Join(t.TempDir(), "recorded.yaml")
	srv := newTestServer(t)
	srv.SetRecordConfig(RecordConfig{
		Upstream:   upstream.URL,
		OutputPath: tmp,
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/ping", nil)
		w := httptest.NewRecorder()
		srv.ServeMock(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d", i, w.Code)
		}
	}

	data, _ := os.ReadFile(tmp)
	var cfg rule.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("recorded YAML invalid: %v", err)
	}
	if len(cfg.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(cfg.Rules))
	}
}

func TestRecordAndProxyStdout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	srv := newTestServer(t)
	srv.SetRecordConfig(RecordConfig{
		Upstream: upstream.URL,
	})

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	srv.ServeMock(rec, req)

	w.Close()
	os.Stdout = old

	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "path: /test") {
		t.Errorf("stdout missing recorded rule:\n%s", string(out))
	}
}

func TestRecordedRulesReplay(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"from":"upstream"}`))
	}))
	defer upstream.Close()

	tmp := filepath.Join(t.TempDir(), "recorded.yaml")
	srv := newTestServer(t)
	srv.SetRecordConfig(RecordConfig{
		Upstream:   upstream.URL,
		OutputPath: tmp,
	})

	req := httptest.NewRequest("GET", "/data", nil)
	w := httptest.NewRecorder()
	srv.ServeMock(w, req)

	data, _ := os.ReadFile(tmp)

	replaySrv := newTestServerWithConfig(t, data)
	req2 := httptest.NewRequest("GET", "/data", nil)
	w2 := httptest.NewRecorder()
	replaySrv.ServeMock(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", w2.Code)
	}
	if w2.Body.String() != `{"from":"upstream"}` {
		t.Fatalf("replay body = %q", w2.Body.String())
	}
}

func TestIsTextContent(t *testing.T) {
	tests := []struct {
		ct   string
		text bool
	}{
		{"text/plain", true},
		{"text/html; charset=utf-8", true},
		{"application/json", true},
		{"application/xml", true},
		{"application/x-www-form-urlencoded", true},
		{"application/octet-stream", false},
		{"image/png", false},
		{"application/protobuf", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isTextContent(tt.ct)
		if got != tt.text {
			t.Errorf("isTextContent(%q) = %v, want %v", tt.ct, got, tt.text)
		}
	}
}

func TestBinaryBodyFlagged(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte{0x00, 0x01, 0x02, 0x03})
	}))
	defer upstream.Close()

	tmp := filepath.Join(t.TempDir(), "recorded.yaml")
	srv := newTestServer(t)
	srv.SetRecordConfig(RecordConfig{
		Upstream:   upstream.URL,
		OutputPath: tmp,
	})

	req := httptest.NewRequest("GET", "/binary", nil)
	w := httptest.NewRecorder()
	srv.ServeMock(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	data, _ := os.ReadFile(tmp)
	yamlStr := string(data)
	if !strings.Contains(yamlStr, "[binary, 4 bytes]") {
		t.Errorf("binary body not flagged:\n%s", yamlStr)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &rule.Config{}
	cfg.Rules = []rule.Rule{}
	j := NewJournal()
	return NewServer(cfg, "", j, false)
}

func newTestServerWithConfig(t *testing.T, yamlData []byte) *Server {
	t.Helper()
	var cfg rule.Config
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}
	j := NewJournal()
	return NewServer(&cfg, "", j, false)
}
