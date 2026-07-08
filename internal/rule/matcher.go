package rule

import (
	"net/http"
	"regexp"
	"strings"
)

func Match(rule *Rule, r *http.Request, body []byte) bool {
	if NormalizeMethod(rule.Request.Method) != NormalizeMethod(r.Method) {
		return false
	}

	for k, v := range rule.Request.Headers {
		if r.Header.Get(k) != v {
			return false
		}
	}

	for k, v := range rule.Request.Query {
		if r.URL.Query().Get(k) != v {
			return false
		}
	}

	if rule.Request.Body != nil {
		bodyStr := string(body)
		switch rule.Request.Body.Mode {
		case "exact":
			if bodyStr != rule.Request.Body.Value {
				return false
			}
		case "contains":
			if !strings.Contains(bodyStr, rule.Request.Body.Value) {
				return false
			}
		}
	}

	return PathMatches(rule.Request.PathMode, rule.Request.Path, r.URL.Path)
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
