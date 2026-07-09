package rule_test

import (
	"os"
	"testing"
	"time"

	. "github.com/flipmorsch/mock-server/internal/rule"
)

func TestLoadConfigValid(t *testing.T) {
	yaml := `
listen: "127.0.0.1:9090"
rules:
  - name: "get users"
    request:
      method: GET
      path: /users
      path_mode: exact
    response:
      status: 200
      headers:
        content-type: application/json
      body: |
        [{"id": 1}]

  - name: "default"
    request:
      method: GET
      path: /
      path_mode: exact
    response:
      status: 200
      body: OK
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Listen != "127.0.0.1:9090" {
		t.Errorf("listen = %q, want %q", cfg.Listen, "127.0.0.1:9090")
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(cfg.Rules))
	}

	r0 := cfg.Rules[0]
	if r0.Request.Method != "GET" {
		t.Errorf("rule 0 method = %q, want GET", r0.Request.Method)
	}
	if r0.Request.Path != "/users" {
		t.Errorf("rule 0 path = %q, want /users", r0.Request.Path)
	}
	if r0.Response.Status != 200 {
		t.Errorf("rule 0 status = %d, want 200", r0.Response.Status)
	}
	if ct := r0.Response.Headers["Content-Type"]; ct != "application/json" {
		t.Errorf("rule 0 Content-Type = %q, want application/json", ct)
	}
	if r0.Response.Body != `[{"id": 1}]`+"\n" {
		t.Errorf("rule 0 body = %q", r0.Response.Body)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	yaml := `
rules:
  - name: "minimal"
    request:
      method: GET
      path: /
    response:
      status: 200
      body: hi
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Listen != "" {
		t.Errorf("listen = %q, want empty", cfg.Listen)
	}
	if addr := cfg.ListenAddr(); addr != "127.0.0.1:8080" {
		t.Errorf("listenAddr = %q, want 127.0.0.1:8080", addr)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(cfg.Rules))
	}
}

func TestLoadConfigMissingMethod(t *testing.T) {
	yaml := `
rules:
  - name: "bad"
    request:
      path: /
    response:
      status: 200
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for missing method")
	}
}

func TestLoadConfigMissingPath(t *testing.T) {
	yaml := `
rules:
  - name: "bad"
    request:
      method: GET
    response:
      status: 200
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestLoadConfigBodyAndBodyFileExclusive(t *testing.T) {
	yaml := `
rules:
  - name: "bad"
    request:
      method: GET
      path: /
    response:
      status: 200
      body: inline
      body_file: external.txt
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for body+body_file")
	}
}

func TestLoadConfigUnsupportedPathMode(t *testing.T) {
	yaml := `
rules:
  - name: "bad"
    request:
      method: GET
      path: /
      path_mode: glob
    response:
      status: 200
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for unsupported path_mode")
	}
}

func TestLoadConfigPrefixPathMode(t *testing.T) {
	yaml := `
rules:
  - name: "prefix rule"
    request:
      method: GET
      path: /api
      path_mode: prefix
    response:
      status: 200
      body: ok
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Rules[0].Request.PathMode != "prefix" {
		t.Errorf("path_mode = %q, want prefix", cfg.Rules[0].Request.PathMode)
	}
}

func TestLoadConfigRegexPathMode(t *testing.T) {
	yaml := `
rules:
  - name: "regex rule"
    request:
      method: GET
      path: "^/users/\\d+$"
      path_mode: regex
    response:
      status: 200
      body: ok
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Rules[0].Request.PathMode != "regex" {
		t.Errorf("path_mode = %q, want regex", cfg.Rules[0].Request.PathMode)
	}
}

func TestLoadConfigRegexInvalid(t *testing.T) {
	yaml := `
rules:
  - name: "bad regex"
    request:
      method: GET
      path: "[unclosed"
      path_mode: regex
    response:
      status: 200
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

func TestLoadConfigPatternPathMode(t *testing.T) {
	yaml := `
rules:
  - name: "pattern rule"
    request:
      method: GET
      path: /users/{id}
      path_mode: pattern
    response:
      status: 200
      body: ok
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Rules[0].Request.PathMode != "pattern" {
		t.Errorf("path_mode = %q, want pattern", cfg.Rules[0].Request.PathMode)
	}
}

func TestLoadConfigPatternInvalid(t *testing.T) {
	for _, path := range []string{"/users/{", "/users/{}", "/users/{1bad}", "/x/{id}/y/{id}"} {
		yaml := `
rules:
  - name: "bad pattern"
    request:
      method: GET
      path: "` + path + `"
      path_mode: pattern
    response:
      status: 200
`
		tmp := writeTemp(t, "config*.yaml", yaml)
		if _, err := LoadConfig(tmp); err == nil {
			t.Errorf("expected error for invalid pattern %q", path)
		}
		os.Remove(tmp)
	}
}

func TestLoadConfigRequestHeaderCanonicalization(t *testing.T) {
	yaml := `
rules:
  - name: "test"
    request:
      method: GET
      path: /
      headers:
        content-type: application/json
        x-custom-header: value
    response:
      status: 200
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h := cfg.Rules[0].Request.Headers
	if h["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", h["Content-Type"])
	}
	if h["X-Custom-Header"] != "value" {
		t.Errorf("X-Custom-Header = %q, want value", h["X-Custom-Header"])
	}
}

func TestLoadConfigBodyFile(t *testing.T) {
	bodyContent := `{"from": "file"}`
	bodyFile := writeTemp(t, "body*.json", bodyContent)
	defer os.Remove(bodyFile)

	yaml := `
rules:
  - name: "from file"
    request:
      method: GET
      path: /data
    response:
      status: 200
      body_file: ` + bodyFile + `
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Rules[0].Response.Body != "" {
		t.Errorf("body should stay empty (file is read at serve time), got %q", cfg.Rules[0].Response.Body)
	}
	if cfg.Rules[0].Response.BodyFile != bodyFile {
		t.Errorf("BodyFile = %q, want %q (reference must survive load)", cfg.Rules[0].Response.BodyFile, bodyFile)
	}
}

func TestLoadConfigBodyFileNotFound(t *testing.T) {
	yaml := `
rules:
  - name: "bad"
    request:
      method: GET
      path: /
    response:
      status: 200
      body_file: /nonexistent/path/file.txt
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for missing body_file")
	}
}

func TestLoadConfigHeaderCanonicalization(t *testing.T) {
	yaml := `
rules:
  - name: "test"
    request:
      method: GET
      path: /
    response:
      status: 200
      headers:
        content-type: application/json
        x-custom-header: value
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h := cfg.Rules[0].Response.Headers
	if h["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", h["Content-Type"])
	}
	if h["X-Custom-Header"] != "value" {
		t.Errorf("X-Custom-Header = %q, want value", h["X-Custom-Header"])
	}
}

func TestLoadConfigDelay(t *testing.T) {
	yaml := `
rules:
  - name: "slow"
    request:
      method: GET
      path: /slow
    response:
      status: 200
      delay: 500ms
      body: ok
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Rules[0].Response.Delay != "500ms" {
		t.Errorf("delay = %q, want 500ms", cfg.Rules[0].Response.Delay)
	}
	if cfg.Rules[0].Response.DelayDuration != 500*time.Millisecond {
		t.Errorf("delayDuration = %v, want 500ms", cfg.Rules[0].Response.DelayDuration)
	}
}

func TestLoadConfigDelayInvalid(t *testing.T) {
	yaml := `
rules:
  - name: "bad delay"
    request:
      method: GET
      path: /
    response:
      status: 200
      delay: not-a-duration
`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for invalid delay")
	}
}

func TestLoadConfigMalformedYaml(t *testing.T) {
	tmp := writeTemp(t, "config*.yaml", "this: is: not: valid: yaml: [")
	defer os.Remove(tmp)

	_, err := LoadConfig(tmp)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestListenAddrDefault(t *testing.T) {
	cfg := &Config{}
	if addr := cfg.ListenAddr(); addr != "127.0.0.1:8080" {
		t.Errorf("default listenAddr = %q, want 127.0.0.1:8080", addr)
	}
}

func TestListenAddrCustom(t *testing.T) {
	cfg := &Config{Listen: "0.0.0.0:3000"}
	if addr := cfg.ListenAddr(); addr != "0.0.0.0:3000" {
		t.Errorf("custom listenAddr = %q, want 0.0.0.0:3000", addr)
	}
}

func TestLoadConfigEmptyRules(t *testing.T) {
	yaml := `listen: "127.0.0.1:9090"`
	tmp := writeTemp(t, "config*.yaml", yaml)
	defer os.Remove(tmp)

	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(cfg.Rules))
	}
}

func writeTemp(t *testing.T, pattern, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatalf("write temp: %v", err)
	}
	f.Close()
	return f.Name()
}
