package main

import (
	"context"
	"net/http"
)

func shellPage(srv *Server, w http.ResponseWriter) {
	rules := srv.WorkingCopy()
	unsaved := srv.HasUnsaved()
	Shell(rules, unsaved).Render(context.Background(), w)
}

func renderSidebar(srv *Server, w http.ResponseWriter) {
	rules := srv.WorkingCopy()
	unsaved := srv.HasUnsaved()
	Sidebar(rules, unsaved).Render(context.Background(), w)
}

func renderRuleEditor(srv *Server, rule Rule, w http.ResponseWriter) {
	RuleEditor(rule).Render(context.Background(), w)
}

func renderJournal(entries []JournalEntry, w http.ResponseWriter) {
	JournalPanel(entries).Render(context.Background(), w)
}

func renderSettings(srv *Server, w http.ResponseWriter) {
	cfg := srv.GetConfig()
	SettingsPanel(cfg).Render(context.Background(), w)
}

func renderTemplatePreview(result string, err string, w http.ResponseWriter) {
	TemplatePreview(result, err).Render(context.Background(), w)
}

func executeTemplateForPreview(body string, probe probeRequest) (string, error) {
	return executeTemplate(body, nil, []byte(probe.Body), &Journal{})
}

func renderDryRunResult(matched bool, w http.ResponseWriter) {
	DryRunResult(matched).Render(context.Background(), w)
}

func renderProbeResult(result probeResult, w http.ResponseWriter) {
	ProbeResult(result).Render(context.Background(), w)
}
