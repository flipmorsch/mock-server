package main

import (
	"net/http/httptest"
	"testing"
)

func TestMatchExactMethodAndPath(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method:   "GET",
			Path:     "/users",
			PathMode: "exact",
		},
	}

	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{"exact match", "GET", "/users", true},
		{"wrong method", "POST", "/users", false},
		{"wrong path", "GET", "/other", false},
		{"case insensitive method", "get", "/users", true},
		{"trailing slash mismatch", "GET", "/users/", false},
		{"subpath no match", "GET", "/users/42", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			got := match(rule, req)
			if got != tt.want {
				t.Errorf("match(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchDefaultPathModeIsExact(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method: "GET",
			Path:   "/users",
		},
	}

	req := httptest.NewRequest("GET", "/users", nil)
	if !match(rule, req) {
		t.Error("match should default to exact path mode")
	}

	req = httptest.NewRequest("GET", "/users/42", nil)
	if match(rule, req) {
		t.Error("match should not match subpath in exact mode")
	}
}

func TestMatchPrefixPath(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method:   "GET",
			Path:     "/users",
			PathMode: "prefix",
		},
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exact match", "/users", true},
		{"subpath", "/users/42", true},
		{"deep subpath", "/users/42/profile", true},
		{"wrong prefix", "/other/42", false},
		{"partial segment", "/users-extra", false},
		{"root", "/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			got := match(rule, req)
			if got != tt.want {
				t.Errorf("match(GET %s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchRegexPath(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method:   "GET",
			Path:     `^/users/\d+$`,
			PathMode: "regex",
		},
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"numeric id", "/users/42", true},
		{"multi-digit", "/users/12345", true},
		{"alphabetic id", "/users/abc", false},
		{"no id", "/users", false},
		{"subpath", "/users/42/profile", false},
		{"wrong path", "/other/42", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			got := match(rule, req)
			if got != tt.want {
				t.Errorf("match(GET %s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchMethodNormalization(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method: "  post  ",
			Path:   "/submit",
		},
	}

	req := httptest.NewRequest("POST", "/submit", nil)
	if !match(rule, req) {
		t.Error("trimmed method should match")
	}
}

func TestMatchQueryStringIgnored(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method: "GET",
			Path:   "/users",
		},
	}

	req := httptest.NewRequest("GET", "/users?page=2", nil)
	if !match(rule, req) {
		t.Error("query string should not affect exact path matching")
	}
}

func TestFirstMatchWins(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name: "first",
				Request: Request{
					Method: "GET",
					Path:   "/resource",
				},
				Response: Response{Status: 200, Body: "first"},
			},
			{
				Name: "second",
				Request: Request{
					Method: "GET",
					Path:   "/resource",
				},
				Response: Response{Status: 200, Body: "second"},
			},
		},
	}

	h := newHandler(cfg)
	req := httptest.NewRequest("GET", "/resource", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Body.String() != "first" {
		t.Errorf("body = %q, want %q (first match wins)", w.Body.String(), "first")
	}
}
