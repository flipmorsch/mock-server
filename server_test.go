package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "mock-server/internal/rule"
	. "mock-server/internal/server"
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

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
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req := httptest.NewRequest("GET", "/anything", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404 for empty config", w.Code)
	}
}

func TestServerMethodMisMatch(t *testing.T) {
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

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
					DelayDuration: 50 * time.Millisecond,
				},
			},
		},
	}

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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
	f := filepath.Join(t.TempDir(), "body.tpl")
	if err := os.WriteFile(f, []byte("path={{.Path}} method={{.Method}}"), 0644); err != nil {
		t.Fatal(err)
	}
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
					BodyFile: f,
				},
			},
		},
	}

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
	req := httptest.NewRequest("GET", "/tplfile", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Body.String() != "path=/tplfile method=GET" {
		t.Errorf("body = %q, want 'path=/tplfile method=GET'", w.Body.String())
	}

	if err := os.WriteFile(f, []byte("edited {{.Method}}"), 0644); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/tplfile", nil))
	if w.Body.String() != "edited GET" {
		t.Errorf("fixture edits should apply live, got %q", w.Body.String())
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
					DelayDuration: 10 * time.Millisecond,
					Body:          "{{.Method}}",
				},
			},
		},
	}

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}
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

	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req := httptest.NewRequest("POST", "/submit", strings.NewReader("wrong body"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404 for non-matching body", w.Code)
	}
}

func TestAdminRequestsEmpty(t *testing.T) {
	cfg := &Config{Rules: []Rule{}}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req := httptest.NewRequest("GET", "/__admin/requests", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "[]\n" {
		t.Errorf("body = %q, want []", w.Body.String())
	}
}

func TestAdminRequestsWithEntries(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name:     "test",
				Request:  Request{Method: "GET", Path: "/hello"},
				Response: Response{Status: 200, Body: "hi"},
			},
		},
	}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req := httptest.NewRequest("GET", "/hello?x=1", nil)
	req.Header.Set("X-Test", "val")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	adminReq := httptest.NewRequest("GET", "/__admin/requests", nil)
	adminW := httptest.NewRecorder()
	h.ServeHTTP(adminW, adminReq)

	if adminW.Code != 200 {
		t.Errorf("status = %d, want 200", adminW.Code)
	}
	body := adminW.Body.String()
	if !strings.Contains(body, `"method":"GET"`) {
		t.Error("journal should contain method")
	}
	if !strings.Contains(body, `"path":"/hello"`) {
		t.Error("journal should contain path")
	}
	if !strings.Contains(body, `"matched":"test"`) {
		t.Error("journal should contain matched rule name")
	}
}

func TestAdminRequestsUnmatched(t *testing.T) {
	cfg := &Config{Rules: []Rule{}}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req := httptest.NewRequest("GET", "/nope", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	adminReq := httptest.NewRequest("GET", "/__admin/requests", nil)
	adminW := httptest.NewRecorder()
	h.ServeHTTP(adminW, adminReq)

	body := adminW.Body.String()
	if !strings.Contains(body, `"matched":""`) {
		t.Error("unmatched requests should have empty matched field")
	}
	if !strings.Contains(body, `"status":404`) {
		t.Error("unmatched requests should have status 404")
	}
}

func TestAdminRequestsCount(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{Name: "a", Request: Request{Method: "GET", Path: "/a"}, Response: Response{Status: 200}},
			{Name: "b", Request: Request{Method: "POST", Path: "/b"}, Response: Response{Status: 201}},
		},
	}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/a", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/b", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/a", nil))

	adminReq := httptest.NewRequest("GET", "/__admin/requests/count?method=GET", nil)
	adminW := httptest.NewRecorder()
	h.ServeHTTP(adminW, adminReq)

	if !strings.Contains(adminW.Body.String(), `"count":2`) {
		t.Errorf("count for GET should be 2, got %s", adminW.Body.String())
	}
}

func TestAdminRequestsClear(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{Name: "a", Request: Request{Method: "GET", Path: "/a"}, Response: Response{Status: 200}},
		},
	}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/a", nil))

	clearReq := httptest.NewRequest("DELETE", "/__admin/requests", nil)
	clearW := httptest.NewRecorder()
	h.ServeHTTP(clearW, clearReq)

	if clearW.Code != 200 {
		t.Errorf("clear status = %d, want 200", clearW.Code)
	}

	listReq := httptest.NewRequest("GET", "/__admin/requests", nil)
	listW := httptest.NewRecorder()
	h.ServeHTTP(listW, listReq)

	if listW.Body.String() != "[]\n" {
		t.Errorf("journal should be empty after clear, got %s", listW.Body.String())
	}
}

func TestAdminRequestsFilterByPathMode(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{Name: "users", Request: Request{Method: "GET", Path: "/users", PathMode: "exact"}, Response: Response{Status: 200}},
		},
	}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/users", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/users/42", nil))

	prefixReq := httptest.NewRequest("GET", "/__admin/requests/count?path=/users&path_mode=prefix", nil)
	prefixW := httptest.NewRecorder()
	h.ServeHTTP(prefixW, prefixReq)
	if !strings.Contains(prefixW.Body.String(), `"count":2`) {
		t.Errorf("prefix count should be 2, got %s", prefixW.Body.String())
	}

	exactReq := httptest.NewRequest("GET", "/__admin/requests/count?path=/users&path_mode=exact", nil)
	exactW := httptest.NewRecorder()
	h.ServeHTTP(exactW, exactReq)
	if !strings.Contains(exactW.Body.String(), `"count":1`) {
		t.Errorf("exact count should be 1, got %s", exactW.Body.String())
	}
}

func TestAdminRequestsFilterByHeader(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{Name: "test", Request: Request{Method: "POST", Path: "/submit"}, Response: Response{Status: 200}},
		},
	}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req1 := httptest.NewRequest("POST", "/submit", nil)
	req1.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req1)

	req2 := httptest.NewRequest("POST", "/submit", nil)
	req2.Header.Set("Content-Type", "text/plain")
	h.ServeHTTP(httptest.NewRecorder(), req2)

	filterReq := httptest.NewRequest("GET", "/__admin/requests/count?header_Content-Type=application/json", nil)
	filterW := httptest.NewRecorder()
	h.ServeHTTP(filterW, filterReq)

	if !strings.Contains(filterW.Body.String(), `"count":1`) {
		t.Errorf("header filter count should be 1, got %s", filterW.Body.String())
	}
}

func TestAdminRequestsBodyFilter(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{Name: "test", Request: Request{Method: "POST", Path: "/submit"}, Response: Response{Status: 200}},
		},
	}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req1 := httptest.NewRequest("POST", "/submit", strings.NewReader(`{"key":"val"}`))
	h.ServeHTTP(httptest.NewRecorder(), req1)

	req2 := httptest.NewRequest("POST", "/submit", strings.NewReader(`{"other":1}`))
	h.ServeHTTP(httptest.NewRecorder(), req2)

	filterReq := httptest.NewRequest("GET", "/__admin/requests/count?body_mode=contains&body=key", nil)
	filterW := httptest.NewRecorder()
	h.ServeHTTP(filterW, filterReq)

	if !strings.Contains(filterW.Body.String(), `"count":1`) {
		t.Errorf("body contains filter count should be 1, got %s", filterW.Body.String())
	}
}

func TestAdminRequestsUnknownPath(t *testing.T) {
	cfg := &Config{Rules: []Rule{}}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	req := httptest.NewRequest("GET", "/__admin/unknown", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("unknown admin path status = %d, want 404", w.Code)
	}
}

func TestServerTemplateRequestCount(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{
				Name:    "count",
				Request: Request{Method: "GET", Path: "/count"},
				Response: Response{
					Status:   200,
					Template: true,
					Body:     "total={{requestCount}} get={{requestCount \"GET\" \"/count\"}}",
				},
			},
		},
	}
	h := &handler{srv: NewServer(cfg, "", NewJournal(), false)}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/other", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/count", nil))

	req := httptest.NewRequest("GET", "/count", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "total=3") {
		t.Errorf("expected total=3, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "get=2") {
		t.Errorf("expected get=2, got %s", w.Body.String())
	}
}
