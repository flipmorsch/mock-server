package server

import (
	"fmt"
	"testing"

	"mock-server/internal/rule"
)

func TestJournalRingBuffer(t *testing.T) {
	j := NewJournal()
	for i := 0; i < 250; i++ {
		j.Record(JournalEntry{Method: "GET", Path: fmt.Sprintf("/r/%d", i), Status: 200})
	}
	entries := j.Entries(nil)
	if len(entries) != 200 {
		t.Fatalf("expected cap of 200 entries, got %d", len(entries))
	}
	if entries[0].Path != "/r/50" || entries[199].Path != "/r/249" {
		t.Fatalf("oldest entries should be evicted first: got %s .. %s", entries[0].Path, entries[199].Path)
	}
	if entries[199].Seq != 250 {
		t.Fatalf("seq should keep counting past eviction, got %d", entries[199].Seq)
	}
}

func TestCountSoundnessPastRingBuffer(t *testing.T) {
	j := NewJournal()
	for i := 0; i < 250; i++ {
		j.Record(JournalEntry{Method: "get", Path: "/x", Status: 200}) // lowercase on purpose
	}
	// Total, by-method, and by-method+path must count all 250 despite the
	// 200-entry display cap — this is the v1.0.1 bug fix.
	if n := j.Count(nil); n != 250 {
		t.Errorf("total count = %d, want 250", n)
	}
	if n := j.Count(&rule.RequestFilter{Method: "GET"}); n != 250 {
		t.Errorf("by-method count = %d, want 250 (case-insensitive)", n)
	}
	if n := j.Count(&rule.RequestFilter{Method: "GET", Path: "/x"}); n != 250 {
		t.Errorf("by-method+path count = %d, want 250", n)
	}
	if n := j.Count(&rule.RequestFilter{Method: "GET", Path: "/nope"}); n != 0 {
		t.Errorf("non-matching path count = %d, want 0", n)
	}
	if n := len(j.Entries(nil)); n != 200 {
		t.Errorf("display buffer should still cap at 200, got %d", n)
	}
	j.Clear()
	if n := j.Count(nil); n != 0 {
		t.Errorf("Clear must reset tallies, total = %d, want 0", n)
	}
}

func TestRecordRedactsSensitiveHeaders(t *testing.T) {
	j := NewJournal()
	j.Record(JournalEntry{
		Method: "GET", Path: "/x", Status: 200,
		Headers: map[string]string{
			"Authorization": "Bearer secret-token",
			"X-Api-Key":     "abc123",
			"Accept":        "application/json",
		},
	})
	e := j.Entries(nil)[0]
	if e.Headers["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization not redacted: %q", e.Headers["Authorization"])
	}
	if e.Headers["X-Api-Key"] != "[REDACTED]" {
		t.Errorf("X-Api-Key not redacted: %q", e.Headers["X-Api-Key"])
	}
	if e.Headers["Accept"] != "application/json" {
		t.Errorf("non-sensitive header must be preserved: %q", e.Headers["Accept"])
	}
}

func TestJournalSubscribe(t *testing.T) {
	j := NewJournal()
	ch, cancel := j.Subscribe()
	j.Record(JournalEntry{Method: "GET", Path: "/live", Status: 200})
	e := <-ch
	if e.Path != "/live" || e.Seq != 1 {
		t.Fatalf("unexpected entry: %+v", e)
	}
	cancel()
	j.Record(JournalEntry{Method: "GET", Path: "/after", Status: 200})
	select {
	case e := <-ch:
		t.Fatalf("received after cancel: %+v", e)
	default:
	}
}
