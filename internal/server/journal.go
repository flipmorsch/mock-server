package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/flipmorsch/mock-server/internal/rule"
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
	ResponseBody string             `json:"response_body,omitempty"`
	Duration     time.Duration      `json:"duration_ns"`
	Explanations []rule.RuleVerdict `json:"explanations,omitempty"`
	// For a match against a sequenced rule: which element served this request
	// (1-based) and how many there are. Zero when the matched rule isn't
	// sequenced. Answers "why did I get this response on this call?".
	SeqPos   int `json:"seq_pos,omitempty"`
	SeqTotal int `json:"seq_total,omitempty"`
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

	// Monotonic request tallies, independent of the display ring buffer, so
	// counts stay sound past maxEntries. Keyed for the aggregations that
	// requestCount and /__admin/requests/count expose (total, method, path).
	countTotal      int64
	countMethod     map[string]int64 // key: upper(method)
	countPath       map[string]int64 // key: exact path
	countMethodPath map[string]int64 // key: upper(method) + "\x00" + path
}

func NewJournal() *Journal {
	return &Journal{
		subs:            make(map[int]chan JournalEntry),
		countMethod:     make(map[string]int64),
		countPath:       make(map[string]int64),
		countMethodPath: make(map[string]int64),
	}
}

var sensitiveHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
	"Set-Cookie":          true,
	"X-Api-Key":           true,
	"X-Auth-Token":        true,
}

// redactHeaders replaces sensitive header values so secrets never land in the
// journal or the /__admin/ API. Returns a new map; nil in, nil out.
func redactHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if sensitiveHeaders[http.CanonicalHeaderKey(k)] {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}

func (j *Journal) Record(e JournalEntry) {
	if len(e.Body) > maxBodyRecord {
		e.Body = e.Body[:maxBodyRecord]
	}
	if len(e.ResponseBody) > maxBodyRecord {
		e.ResponseBody = e.ResponseBody[:maxBodyRecord]
	}
	e.Headers = redactHeaders(e.Headers)
	e.Timestamp = time.Now()

	j.mu.Lock()
	j.seq++
	e.Seq = j.seq

	m := strings.ToUpper(e.Method)
	j.countTotal++
	j.countMethod[m]++
	j.countPath[e.Path]++
	j.countMethodPath[m+"\x00"+e.Path]++
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
	j.countTotal = 0
	j.countMethod = make(map[string]int64)
	j.countPath = make(map[string]int64)
	j.countMethodPath = make(map[string]int64)
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

// Count returns how many recorded requests match the filter. Total / method /
// exact-path filters use monotonic tallies, so they stay sound regardless of the
// display ring buffer. Richer filters (headers, query, body, prefix/regex path)
// can't be pre-aggregated and are scoped to the retained window (last maxEntries).
func (j *Journal) Count(f *rule.RequestFilter) int {
	if f != nil && (len(f.Headers) > 0 || len(f.Query) > 0 || f.Body != "" ||
		(f.PathMode != "" && f.PathMode != "exact")) {
		return len(j.Entries(f)) // window-scoped; Entries takes the lock itself
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	if f == nil {
		return int(j.countTotal)
	}
	m := strings.ToUpper(f.Method)
	switch {
	case f.Method == "" && f.Path == "":
		return int(j.countTotal)
	case f.Path == "":
		return int(j.countMethod[m])
	case f.Method == "":
		return int(j.countPath[f.Path])
	default:
		return int(j.countMethodPath[m+"\x00"+f.Path])
	}
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
		case "json":
			if !rule.JSONBodyMatches(f.Body, e.Body) {
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
