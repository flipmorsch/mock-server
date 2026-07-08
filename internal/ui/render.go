package ui

import (
	"context"
	"net/http"

	"mock-server/internal/rule"
	"mock-server/internal/server"
)

func ShellPage(srv *server.Server, w http.ResponseWriter) {
	rules := srv.WorkingCopy()
	unsaved := srv.HasUnsaved()
	Shell(rules, unsaved).Render(context.Background(), w)
}

func RenderSidebar(srv *server.Server, w http.ResponseWriter) {
	rules := srv.WorkingCopy()
	unsaved := srv.HasUnsaved()
	Sidebar(rules, unsaved).Render(context.Background(), w)
}

func RenderRuleEditor(srv *server.Server, rl rule.Rule, w http.ResponseWriter) {
	RuleEditor(rl).Render(context.Background(), w)
}

func RenderJournal(entries []server.JournalEntry, w http.ResponseWriter) {
	JournalPanel(entries).Render(context.Background(), w)
}

func RenderSettings(srv *server.Server, w http.ResponseWriter) {
	cfg := srv.GetConfig()
	SettingsPanel(cfg).Render(context.Background(), w)
}

func RenderTemplatePreview(result string, err string, w http.ResponseWriter) {
	TemplatePreview(result, err).Render(context.Background(), w)
}

func ExecuteTemplateForPreview(body string, probe ProbeRequest) (string, error) {
	return rule.ExecuteTemplate(body, nil, []byte(probe.Body), func(*rule.RequestFilter) int64 { return 0 })
}

func RenderDryRunResult(matched bool, w http.ResponseWriter) {
	DryRunResult(matched).Render(context.Background(), w)
}

func RenderProbeResult(result ProbeResult, w http.ResponseWriter) {
	ProbeResultView(result).Render(context.Background(), w)
}
