package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/flipmorsch/mock-server/internal/rule"
	"github.com/flipmorsch/mock-server/internal/server"
)

func newTestUI(t *testing.T, cfgYAML string) (*server.Server, http.HandlerFunc) {
	t.Helper()
	cfg, err := rule.ParseConfig([]byte(cfgYAML))
	if err != nil {
		t.Fatal(err)
	}
	srv := server.NewServer(cfg, "", server.NewJournal(), true)
	return srv, Handler(srv, StaticFS)
}

func submit(t *testing.T, h http.HandlerFunc, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// Creating a sequenced rule through the editor form: the responses JSON must
// round-trip into Responses, and a stable id must be minted (else CheckRule's
// "requires an explicit id" would reject it — the bug the id-before-validate fix
// closes).
func TestCreateSequencedRuleViaForm(t *testing.T) {
	srv, h := newTestUI(t, "")
	form := url.Values{
		"method": {"GET"}, "path": {"/job"}, "path_mode": {"exact"},
		"resp_mode": {"sequence"},
		"responses": {`[{"status":202,"body":"pending"},{"status":200,"body":"done"}]`},
	}
	w := submit(t, h, "POST", "/_ui/api/rules", form)
	if w.Code != http.StatusOK {
		t.Fatalf("create: code %d, body %s", w.Code, w.Body.String())
	}
	rules := srv.WorkingCopy()
	if len(rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if !r.Sequenced() || len(r.Responses) != 2 {
		t.Fatalf("want a 2-element sequence, got %+v", r)
	}
	if r.ID == "" {
		t.Error("sequenced rule must have a minted id")
	}
	if r.Responses[0].Status != 202 || r.Responses[0].Body != "pending" || r.Responses[1].Status != 200 {
		t.Errorf("responses decoded wrong: %+v", r.Responses)
	}
}

// The reported bug: duplicating a sequenced rule dropped its responses because
// ruleFromForm ignored them. Duplicate re-POSTs the editor form, so this must
// now carry the list and mint a fresh id.
func TestDuplicateSequencedRuleCarriesResponses(t *testing.T) {
	srv, h := newTestUI(t, "")
	form := url.Values{
		"method": {"GET"}, "path": {"/job"}, "path_mode": {"exact"},
		"resp_mode": {"sequence"},
		"responses": {`[{"status":202},{"status":200}]`},
	}
	submit(t, h, "POST", "/_ui/api/rules", form) // original
	submit(t, h, "POST", "/_ui/api/rules", form) // duplicate (same form values)
	rules := srv.WorkingCopy()
	if len(rules) != 2 {
		t.Fatalf("want 2 rules, got %d", len(rules))
	}
	if !rules[0].Sequenced() || !rules[1].Sequenced() {
		t.Fatalf("duplicate must carry the sequence: %+v / %+v", rules[0], rules[1])
	}
	if rules[0].ID == rules[1].ID || rules[1].ID == "" {
		t.Errorf("duplicate must get a fresh id (got %q and %q)", rules[0].ID, rules[1].ID)
	}
}

func TestEditSequencedRuleViaForm(t *testing.T) {
	srv, h := newTestUI(t, `rules:
  - id: job
    request: {method: GET, path: /job}
    responses:
      - {status: 202}
      - {status: 200}
`)
	form := url.Values{
		"method": {"GET"}, "path": {"/job"}, "path_mode": {"exact"},
		"resp_mode": {"sequence"},
		"responses": {`[{"status":202},{"status":202},{"status":200}]`}, // grew the list
	}
	w := submit(t, h, "PUT", "/_ui/api/rules/job", form)
	if w.Code != http.StatusOK {
		t.Fatalf("edit: code %d, body %s", w.Code, w.Body.String())
	}
	r := srv.FindRule("job")
	if r == nil || len(r.Responses) != 3 {
		t.Fatalf("want a 3-element sequence after edit, got %+v", r)
	}
	if r.ID != "job" {
		t.Errorf("id must be preserved on edit, got %q", r.ID)
	}
}

// A 1-element sequence is just a single response — the server collapses it so
// the saved YAML stays singular (no spurious responses: with one item).
func TestSequenceSingleElementCollapses(t *testing.T) {
	srv, h := newTestUI(t, "")
	form := url.Values{
		"method": {"GET"}, "path": {"/x"}, "path_mode": {"exact"},
		"resp_mode": {"sequence"},
		"responses": {`[{"status":201,"body":"one"}]`},
	}
	submit(t, h, "POST", "/_ui/api/rules", form)
	r := srv.WorkingCopy()[0]
	if r.Sequenced() {
		t.Fatalf("1-element list should collapse to singular, got %d responses", len(r.Responses))
	}
	if r.Response.Status != 201 || r.Response.Body != "one" {
		t.Errorf("collapsed response wrong: %+v", r.Response)
	}
}

func TestEmptySequenceRejected(t *testing.T) {
	srv, h := newTestUI(t, "")
	form := url.Values{
		"method": {"GET"}, "path": {"/x"}, "path_mode": {"exact"},
		"resp_mode": {"sequence"}, "responses": {`[]`},
	}
	submit(t, h, "POST", "/_ui/api/rules", form)
	if n := len(srv.WorkingCopy()); n != 0 {
		t.Errorf("empty sequence must not create a rule, got %d rules", n)
	}
}

// In single mode a stale responses blob left in the form must be ignored — the
// explicit resp_mode discriminator, not JSON presence, decides the shape.
func TestSingleModeIgnoresStaleResponsesJSON(t *testing.T) {
	srv, h := newTestUI(t, "")
	form := url.Values{
		"method": {"GET"}, "path": {"/x"}, "path_mode": {"exact"},
		"resp_mode": {"single"},
		"status":    {"201"}, "body": {"hi"},
		"responses": {`[{"status":999},{"status":998}]`}, // stale — must not win
	}
	submit(t, h, "POST", "/_ui/api/rules", form)
	r := srv.WorkingCopy()[0]
	if r.Sequenced() {
		t.Fatal("single mode must not produce a sequence from stale JSON")
	}
	if r.Response.Status != 201 || r.Response.Body != "hi" {
		t.Errorf("single response wrong: %+v", r.Response)
	}
}
