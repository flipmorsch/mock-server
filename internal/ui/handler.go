package ui

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flipmorsch/mock-server/internal/rule"
	"github.com/flipmorsch/mock-server/internal/server"
)

func Handler(srv *server.Server, staticFS fs.FS) http.HandlerFunc {
	mux := http.NewServeMux()

	staticHandler := http.FileServerFS(staticFS)
	mux.Handle("GET /_ui/static/", http.StripPrefix("/_ui/", staticHandler))

	mux.HandleFunc("GET /_ui/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Shell(srv.WorkingCopy(), srv.HasUnsaved(), srv.Journal().Entries(nil)).Render(r.Context(), w)
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

	// ---- rule CRUD (form-encoded in, HTML out) --------------------------

	mux.HandleFunc("POST /_ui/api/rules", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		rl, err := ruleFromForm(r)
		if err != nil {
			Editor(rl, true, err.Error(), "").Render(r.Context(), w)
			return
		}
		created, err := srv.CreateRule(rl) // mints an id, then validates with it
		if err != nil {
			Editor(rl, true, err.Error(), "").Render(r.Context(), w)
			return
		}
		notify(w, "Rule created", true)
		Editor(created, false, "", "").Render(r.Context(), w)
	})

	mux.HandleFunc("PUT /_ui/api/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		rl, err := ruleFromForm(r)
		if err != nil {
			rl.ID = id
			Editor(rl, false, err.Error(), "").Render(r.Context(), w)
			return
		}
		// Assign the id before validating: a sequenced rule requires a non-empty
		// id, and this is the stable one it keeps (ADR-0008).
		rl.ID = id
		if err := rule.CheckRule(rl); err != nil {
			Editor(rl, false, err.Error(), "").Render(r.Context(), w)
			return
		}
		updated, ok := srv.UpdateRule(id, rl)
		if !ok {
			http.NotFound(w, r)
			return
		}
		notify(w, "Rule updated", true)
		Editor(*updated, false, "", "").Render(r.Context(), w)
	})

	mux.HandleFunc("DELETE /_ui/api/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !srv.DeleteRule(r.PathValue("id")) {
			http.NotFound(w, r)
			return
		}
		notify(w, "Rule deleted", true)
		w.Header().Set("HX-Trigger-After-Swap", `{"editor-closed":true}`)
	})

	mux.HandleFunc("PUT /_ui/api/rules/reorder", func(w http.ResponseWriter, r *http.Request) {
		var ids []string
		if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !srv.ReorderRules(ids) {
			http.Error(w, "id list must match current rules", http.StatusBadRequest)
			return
		}
		notify(w, "", true)
	})

	mux.HandleFunc("PUT /_ui/api/config", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		srv.UpdateConfig(r.FormValue("listen"))
		notify(w, "Listen address staged — restart after save to apply", true)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		SettingsPanel(srv.GetConfig()).Render(r.Context(), w)
	})

	mux.HandleFunc("POST /_ui/api/save", func(w http.ResponseWriter, r *http.Request) {
		if err := srv.Save(); err != nil {
			toast(w, "Save failed: "+err.Error(), "error")
			return
		}
		notify(w, "Saved to disk", false)
	})

	// ---- field validation (blur) ----------------------------------------

	mux.HandleFunc("POST /_ui/api/validate", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if msg := validateField(r); msg != "" {
			FieldError(msg).Render(r.Context(), w)
		}
	})

	// ---- testing ---------------------------------------------------------

	mux.HandleFunc("POST /_ui/api/test-dry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		rl, err := ruleFromForm(r)
		if err != nil {
			TestError("invalid rule: "+err.Error()).Render(r.Context(), w)
			return
		}
		probe := decodeProbeRequest(r)
		req, err := http.NewRequest(probe.Method, probe.Path, strings.NewReader(probe.Body))
		if err != nil {
			TestError("invalid probe request: "+err.Error()).Render(r.Context(), w)
			return
		}
		for k, v := range probe.Headers {
			req.Header.Set(k, v)
		}
		rv := rule.Explain(&rl, req, []byte(probe.Body))
		DryRunResult(rv).Render(r.Context(), w)
	})

	mux.HandleFunc("POST /_ui/api/test-probe", func(w http.ResponseWriter, r *http.Request) {
		probe := decodeProbeRequest(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		resp, err := sendProbe(srv.ListenAddr(), srv.TLSEnabled(), probe)
		if err != nil {
			TestError("probe failed: "+err.Error()).Render(r.Context(), w)
			return
		}
		ProbeResultView(*resp).Render(r.Context(), w)
	})

	mux.HandleFunc("POST /_ui/api/template-preview", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		probe := decodeProbeRequest(r)
		result, err := executePreview(r.FormValue("body"), probe)
		if err != nil {
			TestError("template error: "+err.Error()).Render(r.Context(), w)
			return
		}
		TemplatePreview(result).Render(r.Context(), w)
	})

	// ---- partials --------------------------------------------------------

	mux.HandleFunc("GET /_ui/partials/rail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Rail(srv.WorkingCopy(), srv.HasUnsaved()).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/journal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		JournalPanel(srv.Journal().Entries(nil)).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/rule-editor/{id}", func(w http.ResponseWriter, r *http.Request) {
		rl := srv.FindRule(r.PathValue("id"))
		if rl == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Editor(*rl, false, "", r.URL.Query().Get("highlight")).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/new-rule", func(w http.ResponseWriter, r *http.Request) {
		rl := rule.Rule{Request: rule.Request{Method: "GET"}, Response: rule.Response{Status: 200}}
		if seq, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64); err == nil {
			if e, ok := srv.Journal().Find(seq); ok {
				rl = ruleFromEntry(e)
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Editor(rl, true, "", "").Render(r.Context(), w)
	})

	mux.HandleFunc("GET /_ui/partials/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		SettingsPanel(srv.GetConfig()).Render(r.Context(), w)
	})

	return mux.ServeHTTP
}

// notify sets HX-Trigger events consumed by the shell: rail refresh, the
// unsaved flag (drives the badge + beforeunload warning), and an optional toast.
func notify(w http.ResponseWriter, toastMsg string, unsaved bool) {
	events := map[string]any{"rail-refresh": true, "unsaved": unsaved}
	if toastMsg != "" {
		events["toast"] = map[string]string{"msg": toastMsg, "type": "success"}
	}
	b, _ := json.Marshal(events)
	w.Header().Set("HX-Trigger", string(b))
}

func toast(w http.ResponseWriter, msg, typ string) {
	b, _ := json.Marshal(map[string]any{"toast": map[string]string{"msg": msg, "type": typ}})
	w.Header().Set("HX-Trigger", string(b))
}

// ---- form decoding -------------------------------------------------------

// ruleFromForm builds a rule from the editor form. The response side is read
// per the explicit resp_mode discriminator (ADR-0008): "sequence" decodes the
// hidden `responses` JSON field (a 1-element list collapses to a singular
// response; anything else stays a list, letting CheckRule reject an empty one);
// otherwise the flat single-response fields are used. Reading only the declared
// mode's fields keeps response/responses mutually exclusive by construction.
func ruleFromForm(r *http.Request) (rule.Rule, error) {
	r.ParseForm()
	rl := rule.Rule{
		Name: r.FormValue("name"),
		Request: rule.Request{
			Method:   r.FormValue("method"),
			Path:     r.FormValue("path"),
			PathMode: r.FormValue("path_mode"),
			Headers:  kvFromForm(r, "reqh"),
			Query:    kvFromForm(r, "reqq"),
		},
	}
	if mode := r.FormValue("body_mode"); mode != "" && mode != "none" {
		rl.Request.Body = &rule.BodyMatch{Mode: mode, Value: r.FormValue("body_match")}
	}

	if r.FormValue("resp_mode") == "sequence" {
		var resps []rule.Response
		if err := json.Unmarshal([]byte(r.FormValue("responses")), &resps); err != nil {
			return rl, fmt.Errorf("invalid responses list: %w", err)
		}
		if len(resps) == 1 {
			rl.Response = resps[0] // a lone element is just a single response
		} else {
			rl.Responses = resps // 0 (rejected by CheckRule) or >= 2 (sequenced)
		}
		return rl, nil
	}

	status, _ := strconv.Atoi(r.FormValue("status"))
	rl.Response = rule.Response{
		Status:   status,
		Headers:  kvFromForm(r, "resph"),
		Body:     r.FormValue("body"),
		BodyFile: r.FormValue("body_file"),
		Delay:    r.FormValue("delay"),
		Template: r.FormValue("template") != "",
	}
	return rl, nil
}

func kvFromForm(r *http.Request, prefix string) map[string]string {
	keys, vals := r.Form[prefix+"_k"], r.Form[prefix+"_v"]
	m := make(map[string]string)
	for i, k := range keys {
		if k == "" || i >= len(vals) {
			continue
		}
		m[k] = vals[i]
	}
	if len(m) == 0 {
		return nil
	}
	return m
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

func validateField(r *http.Request) string {
	switch r.FormValue("field") {
	case "path":
		path := r.FormValue("path")
		if path == "" {
			return "path is required"
		}
		if r.FormValue("path_mode") == "regex" {
			if _, err := regexp.Compile(path); err != nil {
				return "invalid regex: " + err.Error()
			}
		}
	case "delay":
		if d := r.FormValue("delay"); d != "" {
			if _, err := time.ParseDuration(d); err != nil {
				return `invalid duration (e.g. 500ms, 2s)`
			}
		}
	case "status":
		s := r.FormValue("status")
		n, err := strconv.Atoi(s)
		if s == "" || err != nil || n < 100 || n > 599 {
			return "status must be 100-599"
		}
	}
	return ""
}

// ---- probing -------------------------------------------------------------

type ProbeRequest struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    string
}

type ProbeResult struct {
	Status  int
	Headers map[string]string
	Body    string
}

func decodeProbeRequest(r *http.Request) ProbeRequest {
	r.ParseForm()
	p := ProbeRequest{
		Method:  r.FormValue("probe_method"),
		Path:    r.FormValue("probe_path"),
		Headers: parseHeaderLines(r.FormValue("probe_headers")),
		Body:    r.FormValue("probe_body"),
	}
	if p.Method == "" {
		p.Method = "GET"
	}
	if p.Path == "" {
		p.Path = "/"
	}
	return p
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

func executePreview(body string, probe ProbeRequest) (string, error) {
	req, err := http.NewRequest(probe.Method, probe.Path, nil)
	if err != nil {
		return "", err
	}
	for k, v := range probe.Headers {
		req.Header.Set(k, v)
	}
	return rule.ExecuteTemplate(body, req, []byte(probe.Body), func(*rule.RequestFilter) int64 { return 0 })
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
