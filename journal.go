package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type JournalEntry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      string            `json:"query"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Matched    string            `json:"matched"`
	Status     int               `json:"status"`
}

const maxBodyRecord = 64 * 1024

type Journal struct {
	mu      sync.RWMutex
	entries []JournalEntry
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

type RequestFilter struct {
	Method   string
	Path     string
	PathMode string
	Headers  map[string]string
	Query    map[string]string
	BodyMode string
	Body     string
}

func (j *Journal) Entries(filter *RequestFilter) []JournalEntry {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if filter == nil || filter.isEmpty() {
		result := make([]JournalEntry, len(j.entries))
		copy(result, j.entries)
		return result
	}

	var result []JournalEntry
	for _, e := range j.entries {
		if !filter.match(&e) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func (j *Journal) Count(filter *RequestFilter) int {
	return len(j.Entries(filter))
}

func (f *RequestFilter) isEmpty() bool {
	return f.Method == "" && f.Path == "" && len(f.Headers) == 0 && len(f.Query) == 0 && f.Body == ""
}

func (f *RequestFilter) match(e *JournalEntry) bool {
	if f.Method != "" && !strings.EqualFold(f.Method, e.Method) {
		return false
	}
	if f.Path != "" {
		switch f.PathMode {
		case "regex":
			matched, _ := regexp.MatchString(f.Path, e.Path)
			if !matched {
				return false
			}
		case "prefix":
			if e.Path != f.Path && !strings.HasPrefix(e.Path, f.Path+"/") {
				return false
			}
		default:
			if e.Path != f.Path {
				return false
			}
		}
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
	if queryString == "" {
		return false
	}
	for _, pair := range strings.Split(queryString, "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 && kv[0] == key && kv[1] == value {
			return true
		}
		if len(kv) == 1 && kv[0] == key && value == "" {
			return true
		}
	}
	return false
}

func adminHandler(journal *Journal) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/__admin/requests" && r.Method == "GET":
			filter := parseFilter(r)
			entries := journal.Entries(filter)
			if entries == nil {
				entries = []JournalEntry{}
			}
			json.NewEncoder(w).Encode(entries)

		case r.URL.Path == "/__admin/requests/count" && r.Method == "GET":
			filter := parseFilter(r)
			count := journal.Count(filter)
			json.NewEncoder(w).Encode(map[string]int{"count": count})

		case r.URL.Path == "/__admin/requests" && r.Method == "DELETE":
			journal.Clear()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})

		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}
}

func parseFilter(r *http.Request) *RequestFilter {
	q := r.URL.Query()
	f := &RequestFilter{
		Method:   q.Get("method"),
		Path:     q.Get("path"),
		PathMode: q.Get("path_mode"),
		BodyMode: q.Get("body_mode"),
		Body:     q.Get("body"),
		Headers:  make(map[string]string),
		Query:    make(map[string]string),
	}
	for key, vals := range q {
		if len(vals) == 0 {
			continue
		}
		if strings.HasPrefix(key, "header_") && len(key) > 7 {
			f.Headers[key[7:]] = vals[0]
		}
		if strings.HasPrefix(key, "query_") && len(key) > 6 {
			f.Query[key[6:]] = vals[0]
		}
	}
	if len(f.Headers) == 0 {
		f.Headers = nil
	}
	if len(f.Query) == 0 {
		f.Query = nil
	}
	return f
}
