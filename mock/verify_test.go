package mock_test

import (
	"net/http"
	"testing"

	"github.com/flipmorsch/mock-server/mock"
)

func TestVerifyAtLeastAtMost(t *testing.T) {
	m, err := mock.Start(`
rules:
  - request: {method: GET, path: /poll}
    response: {status: 200, body: ok}
`)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	for i := 0; i < 3; i++ {
		resp, err := http.Get(m.URL() + "/poll")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	poll := mock.Match{Method: "GET", Path: "/poll"}
	if err := m.VerifyAtLeast(poll, 3); err != nil {
		t.Errorf("VerifyAtLeast(3): %v", err)
	}
	if err := m.VerifyAtLeast(poll, 4); err == nil {
		t.Error("VerifyAtLeast(4) should fail with only 3 requests")
	}
	if err := m.VerifyAtMost(poll, 3); err != nil {
		t.Errorf("VerifyAtMost(3): %v", err)
	}
	if err := m.VerifyAtMost(poll, 2); err == nil {
		t.Error("VerifyAtMost(2) should fail with 3 requests")
	}
}
