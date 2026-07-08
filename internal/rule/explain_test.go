package rule

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplainReportsFailingDimension(t *testing.T) {
	rl := &Rule{
		ID:   "r1",
		Name: "get user",
		Request: Request{
			Method:  "GET",
			Path:    "/users",
			Headers: map[string]string{"Authorization": "Bearer x"},
		},
	}
	req := httptest.NewRequest("GET", "/users", nil)

	rv := Explain(rl, req, nil)
	if rv.Matched {
		t.Fatal("should not match without Authorization header")
	}
	var fail *Verdict
	for i := range rv.Verdicts {
		if !rv.Verdicts[i].OK {
			if fail != nil {
				t.Fatalf("expected exactly one failing verdict, got more: %+v", rv.Verdicts)
			}
			fail = &rv.Verdicts[i]
		}
	}
	if fail == nil || fail.Dim != "header:Authorization" || fail.Want != "Bearer x" || fail.Got != "" {
		t.Fatalf("wrong failing verdict: %+v", fail)
	}
}

func TestExplainMatchedAgreesWithMatch(t *testing.T) {
	rl := &Rule{Request: Request{Method: "POST", Path: "/api", PathMode: "prefix",
		Body: &BodyMatch{Mode: "contains", Value: "name"}}}
	req := httptest.NewRequest("POST", "/api/users", strings.NewReader(""))
	body := []byte(`{"name":"x"}`)
	if !Match(rl, req, body) || !Explain(rl, req, body).Matched {
		t.Fatal("expected match")
	}
}

func TestNearMissesRanking(t *testing.T) {
	req := httptest.NewRequest("GET", "/users/42", nil)
	rules := []Rule{
		{ID: "a", Name: "wrong everything", Request: Request{Method: "POST", Path: "/orders"}},
		{ID: "b", Name: "near miss", Request: Request{Method: "GET", Path: "/users", PathMode: "prefix",
			Headers: map[string]string{"Authorization": "Bearer x"}}},
		{ID: "c", Name: "method only", Request: Request{Method: "GET", Path: "/health"}},
	}
	var all []RuleVerdict
	for i := range rules {
		all = append(all, Explain(&rules[i], req, nil))
	}

	top := NearMisses(all, 2)
	if len(top) != 2 {
		t.Fatalf("expected 2 near misses, got %d", len(top))
	}
	if top[0].RuleID != "b" {
		t.Fatalf("closest should be 'near miss' (path+method agree), got %q", top[0].RuleName)
	}
	if top[1].RuleID != "c" {
		t.Fatalf("second should be 'method only', got %q", top[1].RuleName)
	}
}
