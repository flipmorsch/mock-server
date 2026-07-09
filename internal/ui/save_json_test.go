package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flipmorsch/mock-server/internal/rule"
	"github.com/flipmorsch/mock-server/internal/server"
)

// newTestUIFile is newTestUI with a real config path on disk, so the JSON save
// path (which writes the file) can be exercised.
func newTestUIFile(t *testing.T, cfgYAML string) (*server.Server, http.HandlerFunc, string) {
	t.Helper()
	cfg, err := rule.ParseConfig([]byte(cfgYAML))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	srv := server.NewServer(cfg, path, server.NewJournal(), true)
	return srv, Handler(srv, StaticFS), path
}

func postJSON(t *testing.T, h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// The authoring island seeds itself from GET /_ui/api/rules (ADR-0010): the whole
// working copy as JSON, in the snake_case shape the client edits and posts back.
func TestSeedRulesJSON(t *testing.T) {
	_, h, _ := newTestUIFile(t, `rules:
  - id: r1
    request: {method: GET, path: /health}
    response: {status: 200, body: OK}
`)
	req := httptest.NewRequest("GET", "/_ui/api/rules", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("seed: code %d", w.Code)
	}
	var cfg rule.Config
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("seed not valid JSON: %v (%s)", err, w.Body.String())
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(cfg.Rules))
	}
	r := cfg.Rules[0]
	if r.Request.Method != "GET" || r.Request.Path != "/health" || r.Response.Status != 200 {
		t.Fatalf("seed fields wrong: %+v", r)
	}
	// snake_case round-trips (json tags), not Go field names.
	if !strings.Contains(w.Body.String(), `"path_mode"`) && strings.Contains(w.Body.String(), `"PathMode"`) {
		t.Errorf("expected snake_case JSON keys, got %s", w.Body.String())
	}
}

// POST /_ui/api/save with a JSON working copy writes the file and swaps the
// serving config (ADR-0010) — the whole copy replaces the previous rules.
func TestSaveConfigJSON(t *testing.T) {
	srv, h, path := newTestUIFile(t, "")
	body := `{"listen":"127.0.0.1:9000","rules":[
		{"id":"a","request":{"method":"GET","path":"/users","path_mode":"exact"},
		 "response":{"status":200,"body":"[]"}}]}`
	w := postJSON(t, h, "/_ui/api/save", body)
	if w.Code != http.StatusOK {
		t.Fatalf("save: code %d, body %s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !strings.Contains(string(data), "/users") {
		t.Errorf("saved file missing rule: %s", data)
	}
	// Serving config swapped: the new rule now matches live traffic.
	req := httptest.NewRequest("GET", "/users", nil)
	matched, _ := srv.MatchRule(req, nil)
	if matched == nil || matched.ID != "a" {
		t.Errorf("serving config not swapped; match=%+v", matched)
	}
}

// A save that fails validation is rejected with 422 and leaves the file unwritten.
func TestSaveConfigJSONInvalid(t *testing.T) {
	_, h, path := newTestUIFile(t, "")
	// Sequenced rule without an id — rejected by Check (the ADR-0007 guard).
	body := `{"rules":[{"request":{"method":"GET","path":"/x"},
		"responses":[{"status":202},{"status":200}]}]}`
	w := postJSON(t, h, "/_ui/api/save", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d (%s)", w.Code, w.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("invalid save must not write the file")
	}
}

func TestSaveConfigMalformedJSON(t *testing.T) {
	_, h, _ := newTestUIFile(t, "")
	w := postJSON(t, h, "/_ui/api/save", `{not json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// A sequenced rule (with the required explicit id) saves, round-trips through the
// seed, and serves its first response.
func TestSaveConfigSequencedRule(t *testing.T) {
	srv, h, _ := newTestUIFile(t, "")
	body := `{"rules":[{"id":"job","request":{"method":"GET","path":"/job","path_mode":"exact"},
		"responses":[{"status":202,"body":"pending"},{"status":200,"body":"done"}]}]}`
	if w := postJSON(t, h, "/_ui/api/save", body); w.Code != http.StatusOK {
		t.Fatalf("save sequenced: code %d, body %s", w.Code, w.Body.String())
	}
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest("GET", "/_ui/api/rules", nil))
	var cfg rule.Config
	json.Unmarshal(rw.Body.Bytes(), &cfg)
	if len(cfg.Rules) != 1 || len(cfg.Rules[0].Responses) != 2 {
		t.Fatalf("want a 2-element sequence after save, got %+v", cfg.Rules)
	}
	m, _ := srv.MatchRule(httptest.NewRequest("GET", "/job", nil), nil)
	if m == nil || !m.Sequenced() || m.Responses[0].Status != 202 {
		t.Errorf("sequenced rule not served: %+v", m)
	}
}

// An empty responses list is rejected (422) and leaves the file unwritten.
func TestSaveConfigEmptySequenceRejected(t *testing.T) {
	_, h, path := newTestUIFile(t, "")
	body := `{"rules":[{"id":"x","request":{"method":"GET","path":"/x"},"responses":[]}]}`
	if w := postJSON(t, h, "/_ui/api/save", body); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for empty sequence, got %d", w.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("rejected save must not write the file")
	}
}
