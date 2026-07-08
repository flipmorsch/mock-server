package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func uiHandler(srv *Server) http.HandlerFunc {
	mux := http.NewServeMux()

	staticHandler := http.FileServerFS(staticFS)
	mux.Handle("GET /_ui/static/", http.StripPrefix("/_ui/", staticHandler))

	mux.HandleFunc("GET /_ui/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ui/" && !strings.HasPrefix(r.URL.Path, "/_ui/static/") && !strings.HasPrefix(r.URL.Path, "/_ui/api/") && !strings.HasPrefix(r.URL.Path, "/_ui/partials/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/_ui/" || r.URL.Path == "/_ui" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			shellPage(srv, w)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("GET /_ui/api/rules", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, srv.WorkingCopy())
	})

	mux.HandleFunc("POST /_ui/api/rules", func(w http.ResponseWriter, r *http.Request) {
		var rule Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeJSON(w, http.StatusBadRequest, mapError("invalid JSON: "+err.Error()))
			return
		}
		created := srv.CreateRule(rule)
		writeJSON(w, http.StatusCreated, created)
	})

	mux.HandleFunc("GET /_ui/api/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rule := srv.FindRule(id)
		if rule == nil {
			writeJSON(w, http.StatusNotFound, mapError("rule not found"))
			return
		}
		writeJSON(w, http.StatusOK, rule)
	})

	mux.HandleFunc("PUT /_ui/api/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var updated Rule
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

	mux.HandleFunc("GET /_ui/api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, srv.GetConfig())
	})

	mux.HandleFunc("PUT /_ui/api/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg Config
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

	mux.HandleFunc("GET /_ui/api/unsaved", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"unsaved": srv.HasUnsaved()})
	})

	mux.HandleFunc("POST /_ui/api/rules/{id}/test-dry", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rule := srv.FindRule(id)
		if rule == nil {
			writeJSON(w, http.StatusNotFound, mapError("rule not found"))
			return
		}
		probe := decodeProbeRequest(r)
		matched := probeMatch(rule, probe)
		writeJSON(w, http.StatusOK, map[string]bool{"matched": matched})
	})

	mux.HandleFunc("POST /_ui/api/rules/{id}/test-probe", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rule := srv.FindRule(id)
		if rule == nil {
			writeJSON(w, http.StatusNotFound, mapError("rule not found"))
			return
		}
		probe := decodeProbeRequest(r)
		resp, err := sendProbe(srv.ListenAddr(), probe)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, mapError("probe failed: "+err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("GET /_ui/api/journal", func(w http.ResponseWriter, r *http.Request) {
		entries := srv.journal.Entries(nil)
		if entries == nil {
			entries = []JournalEntry{}
		}
		writeJSON(w, http.StatusOK, entries)
	})


	mux.HandleFunc("POST /_ui/api/upload", func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, mapError("upload failed: "+err.Error()))
			return
		}
		defer file.Close()

		destPath := srv.ResolveFixturePath(header.Filename)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			writeJSON(w, http.StatusInternalServerError, mapError("mkdir failed: "+err.Error()))
			return
		}
		dst, err := os.Create(destPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, mapError("create file failed: "+err.Error()))
			return
		}
		defer dst.Close()
		if _, err := io.Copy(dst, file); err != nil {
			writeJSON(w, http.StatusInternalServerError, mapError("write failed: "+err.Error()))
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{
			"filename": header.Filename,
			"path":     destPath,
		})
	})

	registerPartials(mux, srv)
	return mux.ServeHTTP
}

type probeRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type probeResult struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func decodeProbeRequest(r *http.Request) probeRequest {
	var p probeRequest
	json.NewDecoder(r.Body).Decode(&p)
	if p.Method == "" {
		p.Method = "GET"
	}
	if p.Path == "" {
		p.Path = "/"
	}
	if p.Headers == nil {
		p.Headers = make(map[string]string)
	}
	return p
}

func probeMatch(rule *Rule, probe probeRequest) bool {
	r, _ := http.NewRequest(probe.Method, probe.Path, strings.NewReader(probe.Body))
	for k, v := range probe.Headers {
		r.Header.Set(k, v)
	}
	return match(rule, r, []byte(probe.Body))
}

func sendProbe(addr string, probe probeRequest) (*probeResult, error) {
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

	return &probeResult{
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
