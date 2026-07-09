package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/flipmorsch/mock-server/internal/rule"
)

func TestSaveFidelity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	fixture := filepath.Join(dir, "big.json")
	if err := os.WriteFile(fixture, []byte(`{"from": "file"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &rule.Config{
		Listen: "127.0.0.1:8080",
		Rules: []rule.Rule{{
			ID:   "11111111-2222-4333-8444-555555555555",
			Name: "minimal",
			Request: rule.Request{
				Method:  "GET",
				Path:    "/ping",
				Headers: map[string]string{},
				Query:   map[string]string{},
			},
			Response: rule.Response{
				Status:  200,
				Headers: map[string]string{},
				Body:    "pong",
				Delay:   "500ms",
			},
		}, {
			ID:   "22222222-3333-4444-8555-666666666666",
			Name: "from file",
			Request: rule.Request{
				Method: "GET",
				Path:   "/big",
			},
			Response: rule.Response{
				Status:   200,
				BodyFile: fixture,
			},
		}},
	}
	s := NewServer(cfg, path, NewJournal(), true)

	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	saved := string(data)

	for _, noise := range []string{"delayduration", "path_mode", `body_file: ""`, "headers: {}", "query: {}", "body: null", "template:"} {
		if strings.Contains(saved, noise) {
			t.Errorf("saved yaml contains noise %q:\n%s", noise, saved)
		}
	}
	if !strings.Contains(saved, "body_file: "+fixture) {
		t.Errorf("body_file reference must survive save:\n%s", saved)
	}
	if strings.Contains(saved, "from\": \"file") {
		t.Errorf("body_file content must not be inlined:\n%s", saved)
	}

	loaded, err := rule.LoadConfig(path)
	if err != nil {
		t.Fatalf("reload saved file: %v", err)
	}
	got := loaded.Rules[0]
	if got.ID != cfg.Rules[0].ID || got.Response.Body != "pong" || got.Response.DelayDuration != 500*time.Millisecond {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if loaded.Rules[1].Response.BodyFile != fixture {
		t.Errorf("body_file lost on round-trip: %+v", loaded.Rules[1].Response)
	}
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("rules:\n  - name: original\n    request: {method: GET, path: /a}\n    response: {status: 200, body: A}\n")
	cfg, err := rule.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, path, NewJournal(), false)

	match := func(p string) *rule.Rule {
		got, _ := s.MatchRule(httptest.NewRequest("GET", p, nil), nil)
		return got
	}

	if got := match("/a"); got == nil || got.Name != "original" {
		t.Fatalf("pre-reload: want original for /a, got %v", got)
	}

	// Successful reload swaps the serving set and assigns IDs.
	write("rules:\n  - name: replacement\n    request: {method: GET, path: /b}\n    response: {status: 200, body: B}\n")
	n, err := s.Reload()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if n != 1 {
		t.Errorf("reload count = %d, want 1", n)
	}
	if got := match("/a"); got != nil {
		t.Errorf("post-reload: /a should no longer match, got %v", got)
	}
	got := match("/b")
	if got == nil || got.Name != "replacement" {
		t.Fatalf("post-reload: want replacement for /b, got %v", got)
	}
	if got.ID == "" {
		t.Error("reloaded rule must get an ID for admin/journal coherence")
	}

	// Invalid reload keeps the last-good rule set unchanged.
	write("rules: [{request: {method: GET}, response: {status: 200}}]") // path missing
	if _, err := s.Reload(); err == nil {
		t.Fatal("expected reload to reject invalid config")
	}
	if got := match("/b"); got == nil || got.Name != "replacement" {
		t.Errorf("failed reload must retain rules: want replacement for /b, got %v", got)
	}
}

func TestNewIDIsUUIDv4(t *testing.T) {
	v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for range 100 {
		if id := newID(); !v4.MatchString(id) {
			t.Fatalf("newID() = %q, not RFC-4122 v4", id)
		}
	}
}
