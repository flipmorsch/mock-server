package rule

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// Verdict is one match dimension's outcome for one rule against one request.
// Dim is machine-oriented: "method", "path", "header:<K>", "query:<k>", "body".
type Verdict struct {
	Dim  string `json:"dim"`
	Want string `json:"want"`
	Got  string `json:"got"`
	OK   bool   `json:"ok"`
}

// RuleVerdict is a rule's full evaluation against one request: every
// dimension is checked (no short-circuit) so misses can be explained.
type RuleVerdict struct {
	RuleID   string    `json:"rule_id"`
	RuleName string    `json:"rule_name"`
	Matched  bool      `json:"matched"`
	Verdicts []Verdict `json:"verdicts"`
}

func Explain(rl *Rule, r *http.Request, body []byte) RuleVerdict {
	rv := RuleVerdict{RuleID: rl.ID, RuleName: rl.Name, Matched: true}
	add := func(dim, want, got string, ok bool) {
		rv.Verdicts = append(rv.Verdicts, Verdict{Dim: dim, Want: want, Got: got, OK: ok})
		if !ok {
			rv.Matched = false
		}
	}

	want := NormalizeMethod(rl.Request.Method)
	add("method", want, r.Method, want == NormalizeMethod(r.Method))

	mode := rl.Request.PathMode
	if mode == "" {
		mode = "exact"
	}
	add("path", mode+" "+rl.Request.Path, r.URL.Path,
		PathMatches(rl.Request.PathMode, rl.Request.Path, r.URL.Path))

	for _, k := range sortedKeys(rl.Request.Headers) {
		got := r.Header.Get(k)
		add("header:"+k, rl.Request.Headers[k], got, got == rl.Request.Headers[k])
	}
	for _, k := range sortedKeys(rl.Request.Query) {
		got := r.URL.Query().Get(k)
		add("query:"+k, rl.Request.Query[k], got, got == rl.Request.Query[k])
	}

	if rl.Request.Body != nil {
		bodyStr := string(body)
		bm := rl.Request.Body.Mode
		if bm == "" {
			bm = "exact"
		}
		ok := bodyStr == rl.Request.Body.Value
		if bm == "contains" {
			ok = strings.Contains(bodyStr, rl.Request.Body.Value)
		}
		add("body", bm+" "+rl.Request.Body.Value, truncate(bodyStr, 120), ok)
	}

	return rv
}

func Match(rl *Rule, r *http.Request, body []byte) bool {
	return Explain(rl, r, body).Matched
}

// NearMisses ranks non-matching verdicts by closeness and keeps the top max.
// Rules that matched nothing are dropped — they are noise, not diagnostics.
func NearMisses(all []RuleVerdict, max int) []RuleVerdict {
	var c []RuleVerdict
	for _, rv := range all {
		if !rv.Matched && rv.score() > 0 {
			c = append(c, rv)
		}
	}
	sort.SliceStable(c, func(i, j int) bool { return c[i].score() > c[j].score() })
	if len(c) > max {
		c = c[:max]
	}
	return c
}

// ponytail: closeness = weighted fraction of dimensions passed, path counts
// double (path agreement is the strongest signal of intent); tune if ranking
// feels off in practice.
func (rv RuleVerdict) score() float64 {
	var total, passed float64
	for _, v := range rv.Verdicts {
		w := 1.0
		if v.Dim == "path" {
			w = 2
		}
		total += w
		if v.OK {
			passed += w
		}
	}
	if total == 0 {
		return 0
	}
	return passed / total
}

func PathMatches(mode, pattern, path string) bool {
	switch mode {
	case "prefix":
		return path == pattern || strings.HasPrefix(path, pattern+"/")
	case "regex":
		matched, _ := regexp.MatchString(pattern, path)
		return matched
	default:
		return path == pattern
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
