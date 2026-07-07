package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerMatchedRule(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "test rule",
				Request: Request{
					Method: "GET",
					Path:   "/hello",
				},
				Response: Response{
					Status: 200,
					Body:   "Hello, World!",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/hello", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "Hello, World!" {
		t.Errorf("body = %q, want %q", w.Body.String(), "Hello, World!")
	}
}

func TestServerNoMatch(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "only rule",
				Request: Request{
					Method: "GET",
					Path:   "/exists",
				},
				Response: Response{Status: 200, Body: "ok"},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/nope", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestServerResponseHeaders(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "json rule",
				Request: Request{
					Method: "GET",
					Path:   "/data",
				},
				Response: Response{
					Status: 200,
					Headers: map[string]string{
						"Content-Type": "application/json",
						"X-Custom":     "value",
					},
					Body: `{"key": "val"}`,
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/data", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if x := w.Header().Get("X-Custom"); x != "value" {
		t.Errorf("X-Custom = %q, want value", x)
	}
}

func TestServerDefaultContentType(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "no headers",
				Request: Request{
					Method: "GET",
					Path:   "/plain",
				},
				Response: Response{
					Status: 200,
					Body:   "plain text",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/plain", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
}

func TestServerPreservesStatusCode(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "created",
				Request: Request{
					Method: "POST",
					Path:   "/items",
				},
				Response: Response{Status: 201, Body: "created"},
			},
			{
				Name: "bad request",
				Request: Request{
					Method: "GET",
					Path:   "/bad",
				},
				Response: Response{Status: 400, Body: "nope"},
			},
			{
				Name: "server error",
				Request: Request{
					Method: "GET",
					Path:   "/error",
				},
				Response: Response{Status: 500, Body: "boom"},
			},
		},
	}

	h := newHandler(cfg)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{"POST", "/items", 201},
		{"GET", "/bad", 400},
		{"GET", "/error", 500},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != tt.want {
			t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, w.Code, tt.want)
		}
	}
}

func TestServerEmptyConfig(t *testing.T) {
	cfg := &Config{}
	h := newHandler(cfg)

	req := httptest.NewRequest("GET", "/anything", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404 for empty config", w.Code)
	}
}

func TestServerMethodMismatch(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "get only",
				Request: Request{
					Method: "GET",
					Path:   "/resource",
				},
				Response: Response{Status: 200, Body: "ok"},
			},
		},
	}

	h := newHandler(cfg)

	req := httptest.NewRequest("POST", "/resource", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("POST to GET-only rule: status = %d, want 404", w.Code)
	}
}

func TestServerDelay(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "slow",
				Request: Request{
					Method: "GET",
					Path:   "/slow",
				},
				Response: Response{
					Status:        200,
					Body:          "delayed",
					delayDuration: 50 * time.Millisecond,
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/slow", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	h.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "delayed" {
		t.Errorf("body = %q, want delayed", w.Body.String())
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("elapsed = %v, want at least 50ms", elapsed)
	}
}

func TestServerTemplateMethodAndPath(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "echo",
				Request: Request{
					Method: "GET",
					Path:   "/echo",
				},
				Response: Response{
					Status:   200,
					Template: true,
					Body:     "{{.Method}} {{.Path}}",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/echo?foo=bar", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Body.String() != "GET /echo" {
		t.Errorf("body = %q, want 'GET /echo'", w.Body.String())
	}
}

func TestServerTemplateBodyEcho(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "echo body",
				Request: Request{
					Method: "POST",
					Path:   "/echo",
				},
				Response: Response{
					Status:   200,
					Template: true,
					Body:     "received: {{.Body}}",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("POST", "/echo", strings.NewReader(`{"key":"val"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Body.String() != `received: {"key":"val"}` {
		t.Errorf("body = %q, want 'received: {\"key\":\"val\"}'", w.Body.String())
	}
}

func TestServerTemplateHeaderAndQuery(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "header query",
				Request: Request{
					Method: "GET",
					Path:   "/info",
				},
				Response: Response{
					Status:   200,
					Template: true,
					Body:     "{{.Header \"X-Test\"}} {{.Query \"page\"}}",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/info?page=3", nil)
	req.Header.Set("X-Test", "hello")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Body.String() != "hello 3" {
		t.Errorf("body = %q, want 'hello 3'", w.Body.String())
	}
}

func TestServerTemplateFunctions(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "funcs",
				Request: Request{
					Method: "GET",
					Path:   "/funcs",
				},
				Response: Response{
					Status:   200,
					Template: true,
					Body:     "{{counter}} {{randomInt 1 1}} {{randomString 5}}",
				},
			},
		},
	}

	h := newHandler(cfg)

	req1 := httptest.NewRequest("GET", "/funcs", nil)
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, req1)

	parts1 := strings.SplitN(w1.Body.String(), " ", 3)
	if len(parts1) != 3 {
		t.Fatalf("unexpected body format: %q", w1.Body.String())
	}
	if parts1[0] != "1" {
		t.Errorf("first counter = %q, want 1", parts1[0])
	}
	if parts1[1] != "1" {
		t.Errorf("randomInt(1,1) = %q, want 1", parts1[1])
	}
	if len(parts1[2]) != 5 {
		t.Errorf("randomString(5) length = %d, want 5", len(parts1[2]))
	}

	req2 := httptest.NewRequest("GET", "/funcs", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	parts2 := strings.SplitN(w2.Body.String(), " ", 3)
	if parts2[0] != "2" {
		t.Errorf("second counter = %q, want 2", parts2[0])
	}
}

func TestServerTemplateNoTemplateFlag(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "literal",
				Request: Request{
					Method: "GET",
					Path:   "/literal",
				},
				Response: Response{
					Status: 200,
					Body:   "{{.Method}} is literal",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/literal", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Body.String() != "{{.Method}} is literal" {
		t.Errorf("body = %q, want literal template string", w.Body.String())
	}
}

func TestServerTemplateParseError(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "bad template",
				Request: Request{
					Method: "GET",
					Path:   "/bad",
				},
				Response: Response{
					Status:   200,
					Template: true,
					Body:     "{{.NosuchField}",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/bad", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200 (headers written before template error)", w.Code)
	}
	if w.Body.String() != "" {
		t.Errorf("body should be empty on template error, got %q", w.Body.String())
	}
}

func TestServerTemplateWithBodyFile(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "templated file",
				Request: Request{
					Method: "GET",
					Path:   "/tplfile",
				},
				Response: Response{
					Status:   200,
					Template: true,
					Body:     "path={{.Path}} method={{.Method}}",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/tplfile", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Body.String() != "path=/tplfile method=GET" {
		t.Errorf("body = %q, want 'path=/tplfile method=GET'", w.Body.String())
	}
}

func TestServerDelayAndTemplate(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "slow template",
				Request: Request{
					Method: "GET",
					Path:   "/slowtpl",
				},
				Response: Response{
					Status:        200,
					Template:      true,
					delayDuration: 10 * time.Millisecond,
					Body:          "{{.Method}}",
				},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/slowtpl", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	h.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Body.String() != "GET" {
		t.Errorf("body = %q, want GET", w.Body.String())
	}
	if elapsed < 10*time.Millisecond {
		t.Errorf("elapsed = %v, want at least 10ms", elapsed)
	}
}

func TestServerNoMatchWithBody(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "body rule",
				Request: Request{
					Method: "POST",
					Path:   "/submit",
					Body:   &BodyMatch{Mode: "exact", Value: "expected"},
				},
				Response: Response{Status: 200, Body: "ok"},
			},
		},
	}

	h := newHandler(cfg)

	req := httptest.NewRequest("POST", "/submit", strings.NewReader("wrong body"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404 for non-matching body", w.Code)
	}
}
