package mock_test

import (
	"net/http"
	"testing"

	"github.com/flipmorsch/mock-server/mock"
)

func TestMatchQueryAndHeaders(t *testing.T) {
	m, err := mock.Start(`
rules:
  - request: {method: GET, path: /search}
    response: {status: 200, body: ok}
`)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	req, _ := http.NewRequest("GET", m.URL()+"/search?page=2", nil)
	req.Header.Set("X-Trace", "abc")
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	cases := []struct {
		name string
		m    mock.Match
		want int
	}{
		{"query value match", mock.Match{Method: "GET", Path: "/search", Query: map[string]string{"page": "2"}}, 1},
		{"query value miss", mock.Match{Method: "GET", Path: "/search", Query: map[string]string{"page": "3"}}, 0},
		{"query presence", mock.Match{Method: "GET", Path: "/search", Query: map[string]string{"page": ""}}, 1},
		{"header exact", mock.Match{Method: "GET", Path: "/search", Headers: map[string]string{"X-Trace": "abc"}}, 1},
		{"header value miss", mock.Match{Method: "GET", Path: "/search", Headers: map[string]string{"X-Trace": "nope"}}, 0},
		{"redacted presence", mock.Match{Method: "GET", Path: "/search", Headers: map[string]string{"Authorization": ""}}, 1},
		{"redacted value unmatchable", mock.Match{Method: "GET", Path: "/search", Headers: map[string]string{"Authorization": "Bearer secret"}}, 0},
	}
	for _, c := range cases {
		if got := m.CountMatch(c.m); got != c.want {
			t.Errorf("%s: CountMatch(%s) = %d, want %d", c.name, c.m, got, c.want)
		}
	}
}
