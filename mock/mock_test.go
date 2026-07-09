package mock_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/flipmorsch/mock-server/mock"
)

const cfg = `
rules:
  - name: get user
    request: {method: GET, path: /users/1}
    response:
      status: 200
      headers: {content-type: application/json}
      body: '{"id":1,"name":"Alice"}'
  - name: create user
    request:
      method: POST
      path: /users
      body: {mode: contains, value: '"name"'}
    response: {status: 201, body: created}
`

func TestEmbeddedMock(t *testing.T) {
	m, err := mock.Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// Serves configured responses.
	resp, err := http.Get(m.URL() + "/users/1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "Alice") {
		t.Fatalf("GET /users/1 = %d %q", resp.StatusCode, b)
	}

	// A POST whose body matches the "contains" rule.
	if _, err := http.Post(m.URL()+"/users", "application/json", strings.NewReader(`{"name":"Bob"}`)); err != nil {
		t.Fatal(err)
	}

	// Verification — assert what the code under test sent to the dependency.
	if err := m.VerifyCalled("GET", "/users/1"); err != nil {
		t.Errorf("VerifyCalled: %v", err)
	}
	if err := m.Verify("POST", "/users", 1); err != nil {
		t.Errorf("Verify exact: %v", err)
	}
	if n := m.Count("GET", "/users/1"); n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}

	// A failing assertion diagnoses itself by listing what was received.
	err = m.Verify("DELETE", "/users/1", 1)
	if err == nil {
		t.Fatal("expected Verify to fail for a call that never happened")
	}
	if !strings.Contains(err.Error(), "GET /users/1") {
		t.Errorf("failure should list received requests, got: %v", err)
	}

	// JSON subset body matching in verification.
	if err := m.VerifyMatch(mock.Match{Method: "POST", Path: "/users", JSONBody: `{"name":"Bob"}`}, 1); err != nil {
		t.Errorf("VerifyMatch json subset: %v", err)
	}
	if err := m.VerifyMatch(mock.Match{Method: "POST", Path: "/users", JSONBody: `{"name":"Zed"}`}, 1); err == nil {
		t.Error("VerifyMatch should fail for a non-matching JSON body")
	}

	// Response capture: the journal records what the mock returned.
	var got mock.Request
	for _, rq := range m.Received() {
		if rq.Method == "GET" && rq.Path == "/users/1" {
			got = rq
		}
	}
	if !strings.Contains(got.ResponseBody, "Alice") {
		t.Errorf("response body not captured: %q", got.ResponseBody)
	}

	if n := len(m.Received()); n != 2 {
		t.Errorf("Received len = %d, want 2", n)
	}

	m.Reset()
	if n := len(m.Received()); n != 0 {
		t.Errorf("after Reset, Received len = %d, want 0", n)
	}
}
