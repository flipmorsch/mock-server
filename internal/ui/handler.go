package ui

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"mock-server/internal/rule"
	"mock-server/internal/server"
)

type ProbeRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type ProbeResult struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func Handler(srv *server.Server, staticFS fs.FS) http.HandlerFunc {
	mux := http.NewServeMux()

	staticHandler := http.FileServerFS(staticFS)
	mux.Handle("GET /_ui/static/", http.StripPrefix("/_ui/", staticHandler))

	mux.HandleFunc("GET /_ui/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Shell(srv.WorkingCopy(), srv.HasUnsaved()).Render(r.Context(), w)
	})

	mux.HandleFunc("POST /_ui/api/rules", func(w http.ResponseWriter, r *http.Request) {
		var rl rule.Rule
		if err := json.NewDecoder(r.Body).Decode(&rl); err != nil {
			writeJSON(w, http.StatusBadRequest, mapError("invalid JSON: "+err.Error()))
			return
		}
		created := srv.CreateRule(rl)
		writeJSON(w, http.StatusCreated, created)
	})

	mux.HandleFunc("PUT /_ui/api/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var updated rule.Rule
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			writeJSON(w, http.StatusBadRequest, mapError("invalid JSON: "+err.Error()))
			return
		}
		result, ok := srv.UpdateRule(id, updated)
		if !ok {
			writeJSON(w, http.StatusNotFound, mapError("rule not found"))
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("DELETE /_ui/api/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !srv.DeleteRule(id) {
			writeJSON(w, http.StatusNotFound, mapError("rule not found"))
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	})

	mux.HandleFunc("PUT /_ui/api/rules/reorder", func(w http.ResponseWriter, r *http.Request) {
		var ids []string
		if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
			writeJSON(w, http.StatusBadRequest, mapError("invalid JSON: "+err.Error()))
			return
		}
		if !srv.ReorderRules(ids) {
			writeJSON(w, http.StatusBadRequest, mapError("invalid order: id list must match current rules"))
			return
		}
		writeJSON(w, http.StatusOK, srv.WorkingCopy())
	})

	mux.HandleFunc("PUT /_ui/api/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg rule.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, mapError("invalid JSON: "+err.Error()))
			return
		}
		srv.UpdateConfig(cfg.Listen)
		writeJSON(w, http.StatusOK, srv.GetConfig())
	})

	mux.HandleFunc("POST /_ui/api/save", func(w http.ResponseWriter, r *http.Request) {
		if err := srv.Save(); err != nil {
			writeJSON(w, http.StatusInternalServerError, mapError(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	})

	mux.HandleFunc("POST /_ui/api/rules/{id}/test-dry", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rl := srv.FindRule(id)
		if rl == nil {
			writeJSON(w, http.StatusNotFound, mapError("rule not found"))
			return
		}
		probe := decodeProbeRequest(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		DryRunResult(probeMatch(rl, probe)).Render(r.Context(), w)
	})

	mux.HandleFunc("POST /_ui/api/rules/{id}/test-probe", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rl := srv.FindRule(id)
		if rl == nil {
			writeJSON(w, http.StatusNotFound, mapError("rule not found"))
			return
		}
		probe := decodeProbeRequest(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		resp, err := sendProbe(srv.ListenAddr(), probe)
		if err != nil {
			fmt.Fprintf(w, `<div class="test-result error">probe failed: %s</div>`, html.EscapeString(err.Error()))
			return
		}
		ProbeResultView(*resp).Render(r.Context(), w)
	})

	RegisterPartials(mux, srv)
	return mux.ServeHTTP
}

func RegisterPartials(mux *http.ServeMux, srv *server.Server) {
	mux.HandleFunc("GET /_ui/partials/sidebar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Sidebar(srv.WorkingCopy(), srv.HasUnsaved()).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/rule-editor/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rl := srv.FindRule(id)
		if rl == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		RuleEditor(*rl).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/new-rule", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		RuleEditor(rule.Rule{}).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/journal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		entries := srv.Journal().Entries(nil)
		if entries == nil {
			entries = []server.JournalEntry{}
		}
		JournalPanel(entries).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		SettingsPanel(srv.GetConfig()).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/template-preview/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rl := srv.FindRule(id)
		if rl == nil {
			http.NotFound(w, r)
			return
		}
		if !rl.Response.Template {
			writeJSON(w, http.StatusBadRequest, mapError("template not enabled for this rule"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var probe ProbeRequest
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&probe)
		}
		if probe.Method == "" {
			probe.Method = "GET"
		}
		if probe.Path == "" {
			probe.Path = "/sample"
		}

		result, err := ExecuteTemplateForPreview(rl.Response.Body, probe)
		if err != nil {
			TemplatePreview("", err.Error()).Render(r.Context(), w)
		} else {
			TemplatePreview(result, "").Render(r.Context(), w)
		}
	})
}

func AdminHandler(journal *server.Journal) http.HandlerFunc {
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

func decodeProbeRequest(r *http.Request) ProbeRequest {
	r.ParseForm()
	p := ProbeRequest{
		Method:  r.FormValue("probe_method"),
		Path:    r.FormValue("probe_path"),
		Headers: make(map[string]string),
		Body:    r.FormValue("probe_body"),
	}
	if p.Method == "" {
		p.Method = "GET"
	}
	if p.Path == "" {
		p.Path = "/"
	}
	if h := r.FormValue("probe_headers"); h != "" {
		json.Unmarshal([]byte(h), &p.Headers)
	}
	return p
}

func ExecuteTemplateForPreview(body string, probe ProbeRequest) (string, error) {
	return rule.ExecuteTemplate(body, nil, []byte(probe.Body), func(*rule.RequestFilter) int64 { return 0 })
}

func probeMatch(rl *rule.Rule, probe ProbeRequest) bool {
	r, _ := http.NewRequest(probe.Method, probe.Path, strings.NewReader(probe.Body))
	for k, v := range probe.Headers {
		r.Header.Set(k, v)
	}
	return rule.Match(rl, r, []byte(probe.Body))
}

func sendProbe(addr string, probe ProbeRequest) (*ProbeResult, error) {
	body := strings.NewReader(probe.Body)
	req, err := http.NewRequest(probe.Method, "http://"+addr+probe.Path, body)
	if err != nil {
		return nil, err
	}
	for k, v := range probe.Headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	return &ProbeResult{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    string(respBody),
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("json encode error: %v", err)
		}
	}
}

func mapError(msg string) map[string]string {
	return map[string]string{"error": msg}
}
