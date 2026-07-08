package main

import (
	"encoding/json"
	"net/http"
)
func registerPartials(mux *http.ServeMux, srv *Server) {
	mux.HandleFunc("GET /_ui/partials/sidebar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderSidebar(srv, w)
	})

	mux.HandleFunc("GET /_ui/partials/rule-editor/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rule := srv.FindRule(id)
		if rule == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderRuleEditor(srv, *rule, w)
	})

	mux.HandleFunc("GET /_ui/partials/new-rule", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderRuleEditor(srv, Rule{}, w)
	})

	mux.HandleFunc("GET /_ui/partials/journal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		entries := srv.journal.Entries(nil)
		if entries == nil {
			entries = []JournalEntry{}
		}
		renderJournal(entries, w)
	})

	mux.HandleFunc("GET /_ui/partials/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderSettings(srv, w)
	})

	mux.HandleFunc("GET /_ui/partials/template-preview/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rule := srv.FindRule(id)
		if rule == nil {
			http.NotFound(w, r)
			return
		}
		if !rule.Response.Template {
			writeJSON(w, http.StatusBadRequest, mapError("template not enabled for this rule"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var probe probeRequest
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&probe)
		}
		if probe.Method == "" {
			probe.Method = "GET"
		}
		if probe.Path == "" {
			probe.Path = "/sample"
		}

		result, err := executeTemplateForPreview(rule.Response.Body, probe)
		if err != nil {
			renderTemplatePreview("", err.Error(), w)
		} else {
			renderTemplatePreview(result, "", w)
		}
	})
}
