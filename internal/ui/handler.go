package ui

import (
	"bytes"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/flipmorsch/mock-server/internal/rule"
	"github.com/flipmorsch/mock-server/internal/server"
)

//go:embed static/*
var StaticFS embed.FS

func Handler(srv *server.Server, staticFS fs.FS) http.HandlerFunc {
	mux := http.NewServeMux()

	staticHandler := http.FileServerFS(staticFS)
	mux.Handle("GET /_ui/static/", http.StripPrefix("/_ui/", staticHandler))

	mux.HandleFunc("GET /_ui/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Shell(srv.Journal().Entries(nil)).Render(r.Context(), w)
	})

	// ---- live journal stream -------------------------------------------

	mux.HandleFunc("GET /_ui/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch, cancel := srv.Journal().Subscribe()
		defer cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-ch:
				var buf bytes.Buffer
				JournalRow(e).Render(r.Context(), &buf)
				for _, line := range strings.Split(buf.String(), "\n") {
					fmt.Fprintf(w, "data: %s\n", line)
				}
				fmt.Fprint(w, "\n")
				flusher.Flush()
			}
		}
	})

	// ---- working copy: seed + save (ADR-0010, client-owned) ----------------

	// Seed for the authoring island: the committed rule set as JSON.
	mux.HandleFunc("GET /_ui/api/rules", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(srv.Config())
	})

	// Save: the island POSTs the whole working copy as JSON.
	mux.HandleFunc("POST /_ui/api/save", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var cfg rule.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if err := srv.SaveConfig(cfg); err != nil {
			writeJSONErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// ---- testing (JSON; the island orchestrates, Go engines compute) --------

	mux.HandleFunc("POST /_ui/api/test-dry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req struct {
			Rule  rule.Rule `json:"rule"`
			Probe probeJSON `json:"probe"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		probe := req.Probe.toRequest()
		hreq, err := http.NewRequest(probe.Method, probe.Path, strings.NewReader(probe.Body))
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid probe request: "+err.Error())
			return
		}
		for k, v := range probe.Headers {
			hreq.Header.Set(k, v)
		}
		json.NewEncoder(w).Encode(rule.Explain(&req.Rule, hreq, []byte(probe.Body)))
	})

	mux.HandleFunc("POST /_ui/api/test-probe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var p probeJSON
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		resp, err := sendProbe(srv.ListenAddr(), srv.TLSEnabled(), p.toRequest())
		if err != nil {
			writeJSONErr(w, http.StatusBadGateway, "probe failed: "+err.Error())
			return
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /_ui/api/template-preview", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req struct {
			TplBody      string    `json:"tpl_body"`
			RulePathMode string    `json:"rule_path_mode"`
			RulePath     string    `json:"rule_path"`
			Probe        probeJSON `json:"probe"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		probe := req.Probe.toRequest()
		params := rule.PathParams(req.RulePathMode, req.RulePath, probe.Path)
		out, err := executePreview(req.TplBody, probe, params)
		if err != nil {
			writeJSONErr(w, http.StatusUnprocessableEntity, "template error: "+err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"output": out})
	})

	// rule-from-request: the server builds the pre-filled rule; the island seeds it.
	mux.HandleFunc("GET /_ui/api/rule-from-entry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		seq, err := strconv.ParseInt(r.URL.Query().Get("seq"), 10, 64)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid seq")
			return
		}
		e, ok := srv.Journal().Find(seq)
		if !ok {
			writeJSONErr(w, http.StatusNotFound, "journal entry not found")
			return
		}
		json.NewEncoder(w).Encode(ruleFromEntry(e))
	})

	return mux.ServeHTTP
}

// ruleFromEntry seeds a new rule from a captured request (rule-from-request).
// Headers are deliberately not copied: matching on captured User-Agent and
// friends produces brittle rules; they stay visible in the journal for
// manual cherry-picking.
func ruleFromEntry(e server.JournalEntry) rule.Rule {
	rl := rule.Rule{
		Name: strings.ToLower(e.Method) + " " + e.Path,
		Request: rule.Request{
			Method: e.Method,
			Path:   e.Path,
		},
		Response: rule.Response{Status: 200},
	}
	if q, err := url.ParseQuery(e.Query); err == nil && len(q) > 0 {
		rl.Request.Query = make(map[string]string, len(q))
		for k, vs := range q {
			rl.Request.Query[k] = vs[0]
		}
	}
	if e.Body != "" {
		rl.Request.Body = &rule.BodyMatch{Mode: "exact", Value: e.Body}
	}
	return rl
}

// ---- probing -------------------------------------------------------------

type ProbeRequest struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    string
}

type ProbeResult struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// probeJSON is the island's test-request payload; toRequest parses the raw
// "Key: Value" header lines (reusing parseHeaderLines) and applies defaults.
type probeJSON struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	HeaderText string `json:"headerText"`
	Body       string `json:"body"`
}

func (p probeJSON) toRequest() ProbeRequest {
	pr := ProbeRequest{Method: p.Method, Path: p.Path, Headers: parseHeaderLines(p.HeaderText), Body: p.Body}
	if pr.Method == "" {
		pr.Method = "GET"
	}
	if pr.Path == "" {
		pr.Path = "/"
	}
	return pr
}

func writeJSONErr(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// parseHeaderLines parses "Key: Value" lines, one header per line.
func parseHeaderLines(s string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func sendProbe(addr string, useTLS bool, probe ProbeRequest) (*ProbeResult, error) {
	scheme := "http://"
	client := http.DefaultClient
	if useTLS {
		scheme = "https://"
		// The probe hits the server's own loopback listener, so there is no
		// MITM surface — skip verification, which also covers a user-supplied
		// cert whose SANs don't include the listen address (ADR-0005).
		client = &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ponytail: loopback probe only
		}}
	}
	req, err := http.NewRequest(probe.Method, scheme+addr+probe.Path, strings.NewReader(probe.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range probe.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	return &ProbeResult{Status: resp.StatusCode, Headers: headers, Body: string(respBody)}, nil
}

func executePreview(body string, probe ProbeRequest, params map[string]string) (string, error) {
	req, err := http.NewRequest(probe.Method, probe.Path, nil)
	if err != nil {
		return "", err
	}
	for k, v := range probe.Headers {
		req.Header.Set(k, v)
	}
	return rule.ExecuteTemplate(body, req, []byte(probe.Body), params, func(*rule.RequestFilter) int64 { return 0 })
}

// ---- admin API (programmatic, unchanged contract) --------------------------

func AdminHandler(srv *server.Server) http.HandlerFunc {
	journal := srv.Journal()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/__admin/requests" && r.Method == "GET":
			filter := parseFilter(r)
			entries := journal.Entries(filter)
			if entries == nil {
				entries = []server.JournalEntry{}
			}
			json.NewEncoder(w).Encode(entries)

		case r.URL.Path == "/__admin/requests/count" && r.Method == "GET":
			filter := parseFilter(r)
			json.NewEncoder(w).Encode(map[string]int{"count": journal.Count(filter)})

		case r.URL.Path == "/__admin/requests" && r.Method == "DELETE":
			journal.Clear()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})

		case r.URL.Path == "/__admin/reset" && r.Method == "POST":
			srv.Reset()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "reset"})

		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}
}

func parseFilter(r *http.Request) *rule.RequestFilter {
	q := r.URL.Query()
	f := &rule.RequestFilter{
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
