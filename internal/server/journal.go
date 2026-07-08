package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"mock-server/internal/rule"
)

type JournalEntry struct {
	Seq          int64              `json:"seq"`
	Timestamp    time.Time          `json:"timestamp"`
	Method       string             `json:"method"`
	Path         string             `json:"path"`
	Query        string             `json:"query"`
	Headers      map[string]string  `json:"headers"`
	Body         string             `json:"body"`
	Matched      string             `json:"matched"`
	MatchedID    string             `json:"matched_id,omitempty"`
	Status       int                `json:"status"`
	Explanations []rule.RuleVerdict `json:"explanations,omitempty"`
}

const (
	maxBodyRecord = 64 * 1024
	maxEntries    = 200
)

type Journal struct {
	mu      sync.RWMutex
	entries []JournalEntry
	seq     int64
	subs    map[int]chan JournalEntry
	nextSub int
}

func NewJournal() *Journal {
	return &Journal{subs: make(map[int]chan JournalEntry)}
}

func (j *Journal) Record(e JournalEntry) {
	if len(e.Body) > maxBodyRecord {
		e.Body = e.Body[:maxBodyRecord]
	}
	e.Timestamp = time.Now()

	j.mu.Lock()
	j.seq++
	e.Seq = j.seq
	// ponytail: ring buffer via slice shift; O(n) is nothing at cap 200
	if len(j.entries) >= maxEntries {
		copy(j.entries, j.entries[1:])
		j.entries[len(j.entries)-1] = e
	} else {
		j.entries = append(j.entries, e)
	}
	for _, ch := range j.subs {
		select { // non-blocking: a slow SSE client drops entries, never stalls serving
		case ch <- e:
		default:
		}
	}
	j.mu.Unlock()
}

// Subscribe returns a channel receiving new entries and a cancel func.
// The channel is never closed; after cancel no more sends happen and it is
// garbage-collected with the subscriber.
func (j *Journal) Subscribe() (<-chan JournalEntry, func()) {
	ch := make(chan JournalEntry, 16)
	j.mu.Lock()
	id := j.nextSub
	j.nextSub++
	j.subs[id] = ch
	j.mu.Unlock()
	return ch, func() {
		j.mu.Lock()
		delete(j.subs, id)
		j.mu.Unlock()
	}
}

func (j *Journal) Clear() {
	j.mu.Lock()
	j.entries = nil
	j.mu.Unlock()
}

func (j *Journal) Find(seq int64) (JournalEntry, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	for i := range j.entries {
		if j.entries[i].Seq == seq {
			return j.entries[i], true
		}
	}
	return JournalEntry{}, false
}

func (j *Journal) Entries(filter *rule.RequestFilter) []JournalEntry {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if filter == nil || requestFilterIsEmpty(filter) {
		result := make([]JournalEntry, len(j.entries))
		copy(result, j.entries)
		return result
	}

	var result []JournalEntry
	for _, e := range j.entries {
		if !requestFilterMatch(filter, &e) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func (j *Journal) Count(filter *rule.RequestFilter) int {
	return len(j.Entries(filter))
}

func requestFilterIsEmpty(f *rule.RequestFilter) bool {
	return f.Method == "" && f.Path == "" && len(f.Headers) == 0 && len(f.Query) == 0 && f.Body == ""
}

func requestFilterMatch(f *rule.RequestFilter, e *JournalEntry) bool {
	if f.Method != "" && !strings.EqualFold(f.Method, e.Method) {
		return false
	}
	if f.Path != "" && !rule.PathMatches(f.PathMode, f.Path, e.Path) {
		return false
	}
	for k, v := range f.Headers {
		if hv, ok := e.Headers[http.CanonicalHeaderKey(k)]; !ok || hv != v {
			return false
		}
	}
	for k, v := range f.Query {
		if !queryParamMatches(e.Query, k, v) {
			return false
		}
	}
	if f.Body != "" {
		switch f.BodyMode {
		case "contains":
			if !strings.Contains(e.Body, f.Body) {
				return false
			}
		default:
			if e.Body != f.Body {
				return false
			}
		}
	}
	return true
}

func queryParamMatches(queryString, key, value string) bool {
	q, _ := url.ParseQuery(queryString)
	return slices.Contains(q[key], value)
}
