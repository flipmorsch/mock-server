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
	Timestamp time.Time         `json:"timestamp"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Query     string            `json:"query"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	Matched   string            `json:"matched"`
	Status    int               `json:"status"`
}

const maxBodyRecord = 64 * 1024

type Journal struct {
	mu      sync.RWMutex
	entries []JournalEntry
}

func NewJournal() *Journal {
	return &Journal{}
}

func (j *Journal) Record(method, path, query string, headers map[string]string, body []byte, matched string, status int) {
	bodyStr := string(body)
	if len(bodyStr) > maxBodyRecord {
		bodyStr = bodyStr[:maxBodyRecord]
	}

	j.mu.Lock()
	j.entries = append(j.entries, JournalEntry{
		Timestamp: time.Now(),
		Method:    method,
		Path:      path,
		Query:     query,
		Headers:   headers,
		Body:      bodyStr,
		Matched:   matched,
		Status:    status,
	})
	j.mu.Unlock()
}

func (j *Journal) Clear() {
	j.mu.Lock()
	j.entries = nil
	j.mu.Unlock()
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
