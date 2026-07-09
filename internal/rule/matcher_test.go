package rule_test

import (
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	. "github.com/flipmorsch/mock-server/internal/rule"
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
			got := Match(rule, req, nil)
			if got != tt.want {
				t.Errorf("Match(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
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
	if !Match(rule, req, nil) {
		t.Error("match should default to exact path mode")
	}

	req = httptest.NewRequest("GET", "/users/42", nil)
	if Match(rule, req, nil) {
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
			got := Match(rule, req, nil)
			if got != tt.want {
				t.Errorf("Match(GET %s) = %v, want %v", tt.path, got, tt.want)
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
			got := Match(rule, req, nil)
			if got != tt.want {
				t.Errorf("Match(GET %s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchPatternPath(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method:   "GET",
			Path:     "/users/{id}",
			PathMode: "pattern",
		},
	}
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"one segment", "/users/42", true},
		{"non-numeric segment", "/users/abc", true},
		{"trailing slash", "/users/42/", false},
		{"extra segment", "/users/42/profile", false},
		{"missing segment", "/users", false},
		{"empty segment", "/users/", false},
		{"wrong prefix", "/orders/42", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			if got := Match(rule, req, nil); got != tt.want {
				t.Errorf("Match(GET %s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestPathParams(t *testing.T) {
	tests := []struct {
		name, mode, pattern, path string
		want                      map[string]string
	}{
		{"pattern single", "pattern", "/users/{id}", "/users/42", map[string]string{"id": "42"}},
		{"pattern multi", "pattern", "/u/{uid}/p/{pid}", "/u/7/p/9", map[string]string{"uid": "7", "pid": "9"}},
		{"pattern no match", "pattern", "/users/{id}", "/orders/1", nil},
		{"regex named capture", "regex", `^/users/(?P<id>\d+)$`, "/users/42", map[string]string{"id": "42"}},
		{"exact mode nil", "exact", "/users/1", "/users/1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PathParams(tt.mode, tt.pattern, tt.path)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PathParams(%q, %q, %q) = %v, want %v", tt.mode, tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchHeaders(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method: "POST",
			Path:   "/submit",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
	}

	t.Run("matching header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/submit", nil)
		req.Header.Set("Content-Type", "application/json")
		if !Match(rule, req, nil) {
			t.Error("should match when header value matches")
		}
	})

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/submit", nil)
		if Match(rule, req, nil) {
			t.Error("should not match when required header is missing")
		}
	})

	t.Run("wrong header value", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/submit", nil)
		req.Header.Set("Content-Type", "text/plain")
		if Match(rule, req, nil) {
			t.Error("should not match when header value differs")
		}
	})

	t.Run("multiple headers all match", func(t *testing.T) {
		r := &Rule{
			Request: Request{
				Method: "GET",
				Path:   "/data",
				Headers: map[string]string{
					"Content-Type": "application/json",
					"X-Api-Key":    "secret",
				},
			},
		}
		req := httptest.NewRequest("GET", "/data", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", "secret")
		if !Match(r, req, nil) {
			t.Error("should match when all headers match")
		}
	})

	t.Run("one of multiple headers wrong", func(t *testing.T) {
		r := &Rule{
			Request: Request{
				Method: "GET",
				Path:   "/data",
				Headers: map[string]string{
					"Content-Type": "application/json",
					"X-Api-Key":    "secret",
				},
			},
		}
		req := httptest.NewRequest("GET", "/data", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", "wrong")
		if Match(r, req, nil) {
			t.Error("should not match when one header differs")
		}
	})
}

func TestMatchQueryParams(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method: "GET",
			Path:   "/search",
			Query: map[string]string{
				"q":    "golang",
				"page": "1",
			},
		},
	}

	t.Run("all params match", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/search?q=golang&page=1", nil)
		if !Match(rule, req, nil) {
			t.Error("should match when all query params match")
		}
	})

	t.Run("missing param", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/search?q=golang", nil)
		if Match(rule, req, nil) {
			t.Error("should not match when required param is missing")
		}
	})

	t.Run("wrong param value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/search?q=golang&page=2", nil)
		if Match(rule, req, nil) {
			t.Error("should not match when param value differs")
		}
	})

	t.Run("extra params still match", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/search?q=golang&page=1&sort=asc", nil)
		if !Match(rule, req, nil) {
			t.Error("should match even with extra query params")
		}
	})
}

func TestMatchBody(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "POST",
				Path:   "/submit",
				Body:   &BodyMatch{Mode: "exact", Value: `{"key":"val"}`},
			},
		}
		req := httptest.NewRequest("POST", "/submit", strings.NewReader(`{"key":"val"}`))
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if !Match(rule, req, body) {
			t.Error("should match exact body")
		}
	})

	t.Run("exact mismatch", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "POST",
				Path:   "/submit",
				Body:   &BodyMatch{Mode: "exact", Value: `{"key":"val"}`},
			},
		}
		req := httptest.NewRequest("POST", "/submit", strings.NewReader(`{"other":1}`))
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if Match(rule, req, body) {
			t.Error("should not match different body")
		}
	})

	t.Run("contains match", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "POST",
				Path:   "/submit",
				Body:   &BodyMatch{Mode: "contains", Value: `"name"`},
			},
		}
		req := httptest.NewRequest("POST", "/submit", strings.NewReader(`{"name":"Bob","age":30}`))
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if !Match(rule, req, body) {
			t.Error("should match when body contains value")
		}
	})

	t.Run("contains not found", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "POST",
				Path:   "/submit",
				Body:   &BodyMatch{Mode: "contains", Value: `"missing"`},
			},
		}
		req := httptest.NewRequest("POST", "/submit", strings.NewReader(`{"key":"val"}`))
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if Match(rule, req, body) {
			t.Error("should not match when body does not contain value")
		}
	})

	t.Run("default mode is exact", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "POST",
				Path:   "/submit",
				Body:   &BodyMatch{Value: "hello"},
			},
		}
		req := httptest.NewRequest("POST", "/submit", strings.NewReader("hello"))
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if !Match(rule, req, body) {
			t.Error("default mode should be exact")
		}
	})
}

func TestMatchBodyEdgeCases(t *testing.T) {
	t.Run("exact match with empty body", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "POST",
				Path:   "/submit",
				Body:   &BodyMatch{Mode: "exact", Value: ""},
			},
		}
		req := httptest.NewRequest("POST", "/submit", strings.NewReader(""))
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if !Match(rule, req, body) {
			t.Error("should match empty body exactly")
		}
	})

	t.Run("contains match with empty value", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "POST",
				Path:   "/submit",
				Body:   &BodyMatch{Mode: "contains", Value: ""},
			},
		}
		req := httptest.NewRequest("POST", "/submit", strings.NewReader("anything"))
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if !Match(rule, req, body) {
			t.Error("empty string should be contained in any body")
		}
	})

	t.Run("body match with no body in request", func(t *testing.T) {
		rule := &Rule{
			Request: Request{
				Method: "GET",
				Path:   "/get",
				Body:   &BodyMatch{Mode: "exact", Value: "something"},
			},
		}
		req := httptest.NewRequest("GET", "/get", nil)
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if Match(rule, req, body) {
			t.Error("should not match when rule requires body but request has none")
		}
	})
}

func TestMatchMethodNormalization(t *testing.T) {
	rule := &Rule{
		Request: Request{
			Method: "  post  ",
			Path:   "/submit",
		},
	}

	req := httptest.NewRequest("POST", "/submit", nil)
	if !Match(rule, req, nil) {
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
	if !Match(rule, req, nil) {
		t.Error("query string should not affect exact path matching")
	}
}
