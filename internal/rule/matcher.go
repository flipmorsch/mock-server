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
		var ok bool
		switch bm {
		case "contains":
			ok = strings.Contains(bodyStr, rl.Request.Body.Value)
		case "json":
			ok = JSONBodyMatches(rl.Request.Body.Value, bodyStr)
		default:
			ok = bodyStr == rl.Request.Body.Value
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
	case "pattern":
		// ponytail: compile-per-call like the regex arm; add a cache only if profiling says so.
		matched, _ := regexp.MatchString(patternToRegex(pattern), path)
		return matched
	default:
		return path == pattern
	}
}

// pathParamRe matches a {name} placeholder in a "pattern" path.
var pathParamRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// patternToRegex translates a "pattern" path like /users/{id} into an anchored
// regex ^/users/(?P<id>[^/]+)$: each {name} captures exactly one path segment and
// literal spans are escaped. Mirrors net/http ServeMux {name} wildcards.
func patternToRegex(pattern string) string {
	var b strings.Builder
	b.WriteByte('^')
	last := 0
	for _, loc := range pathParamRe.FindAllStringSubmatchIndex(pattern, -1) {
		b.WriteString(regexp.QuoteMeta(pattern[last:loc[0]]))
		b.WriteString("(?P<")
		b.WriteString(pattern[loc[2]:loc[3]])
		b.WriteString(">[^/]+)")
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(pattern[last:]))
	b.WriteByte('$')
	return b.String()
}

// PathParams extracts the named parameters a "pattern" or "regex" rule path
// captured from the request path. It returns nil for other modes or when the path
// does not match. Called once per request for the winning rule, not in the hot
// per-rule matching loop.
func PathParams(mode, pattern, path string) map[string]string {
	var expr string
	switch mode {
	case "pattern":
		expr = patternToRegex(pattern)
	case "regex":
		expr = pattern
	default:
		return nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil
	}
	m := re.FindStringSubmatch(path)
	if m == nil {
		return nil
	}
	var out map[string]string
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" {
			if out == nil {
				out = make(map[string]string)
			}
			out[name] = m[i]
		}
	}
	return out
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
