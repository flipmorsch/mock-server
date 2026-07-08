package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"mock-server/internal/rule"
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

func TestNewIDIsUUIDv4(t *testing.T) {
	v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for range 100 {
		if id := newID(); !v4.MatchString(id) {
			t.Fatalf("newID() = %q, not RFC-4122 v4", id)
		}
	}
}
