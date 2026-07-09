package mock_test

import (
	"net/http"
	"sort"
	"sync"
	"testing"

	"github.com/flipmorsch/mock-server/mock"
)

func statusOf(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestSequencedResponsesLastSticks(t *testing.T) {
	m, err := mock.Start(`
rules:
  - id: job
    request: {method: GET, path: /job}
    responses:
      - {status: 202}
      - {status: 202}
      - {status: 200}
`)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	want := []int{202, 202, 200, 200, 200} // Nth match → Nth response, last sticks
	for i, w := range want {
		if got := statusOf(t, m.URL()+"/job"); got != w {
			t.Errorf("request %d: status = %d, want %d", i+1, got, w)
		}
	}
}

func TestResetRewindsSequence(t *testing.T) {
	m, err := mock.Start(`
rules:
  - id: job
    request: {method: GET, path: /job}
    responses:
      - {status: 202}
      - {status: 200}
`)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if got := statusOf(t, m.URL()+"/job"); got != 202 {
		t.Fatalf("first: got %d, want 202", got)
	}
	if got := statusOf(t, m.URL()+"/job"); got != 200 {
		t.Fatalf("second: got %d, want 200", got)
	}

	m.Reset()

	if got := statusOf(t, m.URL()+"/job"); got != 202 {
		t.Errorf("after reset: got %d, want 202 (sequence should rewind)", got)
	}
	// Reset also clears the journal.
	if err := m.Verify("GET", "/job", 1); err != nil {
		t.Errorf("journal should show only the post-reset request: %v", err)
	}
}

// Under concurrency, N simultaneous hits to an N-element sequence must each get a
// distinct element — no duplicate, no skip. Runs under -race.
func TestSequencedResponsesConcurrent(t *testing.T) {
	m, err := mock.Start(`
rules:
  - id: seq
    request: {method: GET, path: /seq}
    responses:
      - {status: 201}
      - {status: 202}
      - {status: 203}
      - {status: 204}
      - {status: 205}
`)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	const n = 5
	got := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i] = statusOf(t, m.URL()+"/seq")
		}(i)
	}
	wg.Wait()

	sort.Ints(got)
	for i, want := 0, 201; i < n; i, want = i+1, want+1 {
		if got[i] != want {
			t.Errorf("collected statuses %v, want a permutation of 201..205", got)
			break
		}
	}
}
