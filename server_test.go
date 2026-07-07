package main

import (
	"net/http/httptest"
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
