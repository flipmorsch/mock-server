package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/flipmorsch/mock-server/internal/rule"
)

// A sequenced rule keyed by a stable explicit id keeps its position across a
// SIGHUP reload — an unrelated reload must not rewind a client mid-sequence.
func TestSequencedReloadPreservesIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfgYAML := `rules:
  - id: job
    request: {method: GET, path: /job}
    responses:
      - {status: 202}
      - {status: 203}
      - {status: 200}
`
	if err := os.WriteFile(path, []byte(cfgYAML), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := rule.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, path, NewJournal(), false)

	hit := func() int {
		w := httptest.NewRecorder()
		s.ServeMock(w, httptest.NewRequest("GET", "/job", nil))
		return w.Code
	}

	if got := hit(); got != 202 {
		t.Fatalf("hit 1: got %d, want 202", got)
	}
	if got := hit(); got != 203 {
		t.Fatalf("hit 2: got %d, want 203", got)
	}

	// Reload the same file (same id) — position must survive, so the next hit is
	// the 3rd element, not a rewind to the 1st.
	if _, err := s.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := hit(); got != 200 {
		t.Errorf("hit 3 after reload: got %d, want 200 (index must survive reload)", got)
	}

	// Reset rewinds it back to the first element.
	s.Reset()
	if got := hit(); got != 202 {
		t.Errorf("hit after reset: got %d, want 202", got)
	}
}

// The journal records which sequence element served each request (1-based,
// clamped once exhausted), so the UI can show "seq N/M". A non-sequenced match
// records zeroes.
func TestSequencePositionRecordedInJournal(t *testing.T) {
	cfg, err := rule.ParseConfig([]byte(`rules:
  - id: job
    request: {method: GET, path: /jobs/1}
    responses:
      - {status: 202}
      - {status: 200}
  - name: plain
    request: {method: GET, path: /health}
    response: {status: 200}
`))
	if err != nil {
		t.Fatal(err)
	}
	j := NewJournal()
	s := NewServer(cfg, "", j, false)
	hit := func(path string) {
		s.ServeMock(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	hit("/jobs/1")
	hit("/jobs/1")
	hit("/jobs/1") // exhausted → clamps to last
	hit("/health") // not sequenced

	entries := j.Entries(nil) // oldest first
	want := []struct{ pos, total int }{{1, 2}, {2, 2}, {2, 2}, {0, 0}}
	if len(entries) != len(want) {
		t.Fatalf("want %d entries, got %d", len(want), len(entries))
	}
	for i, e := range entries {
		if e.SeqPos != want[i].pos || e.SeqTotal != want[i].total {
			t.Errorf("entry %d (%s): seq %d/%d, want %d/%d", i, e.Path, e.SeqPos, e.SeqTotal, want[i].pos, want[i].total)
		}
	}
}
