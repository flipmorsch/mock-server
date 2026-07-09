package mock_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/flipmorsch/mock-server/mock"
)

func TestVerifyMatchSelfDiagnoses(t *testing.T) {
	m, err := mock.Start(`
rules:
  - request: {method: POST, path: /pay}
    response: {status: 200, body: ok}
`)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// Code under test sends amount 300; the test expected 500.
	resp, err := http.Post(m.URL()+"/pay", "application/json", strings.NewReader(`{"amount":300}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	err = m.VerifyMatch(mock.Match{Method: "POST", Path: "/pay", JSONBody: `{"amount":500}`}, 1)
	if err == nil {
		t.Fatal("expected VerifyMatch to fail")
	}
	msg := err.Error()
	for _, want := range []string{`body: {"amount":300}`, "JSONBody.amount", "got 300", "want 500"} {
		if !strings.Contains(msg, want) {
			t.Errorf("failure message missing %q\ngot:\n%s", want, msg)
		}
	}
}
