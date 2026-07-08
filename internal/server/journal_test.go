package server

import (
	"fmt"
	"testing"
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
