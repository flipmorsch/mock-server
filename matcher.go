package main

import (
	"net/http"
	"regexp"
	"strings"
)

func match(rule *Rule, r *http.Request, body []byte) bool {
	if normalizeMethod(rule.Request.Method) != normalizeMethod(r.Method) {
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

	mode := rule.Request.PathMode
	if mode == "" {
		mode = "exact"
	}

	switch mode {
	case "exact":
		return rule.Request.Path == r.URL.Path
	case "prefix":
		return r.URL.Path == rule.Request.Path || strings.HasPrefix(r.URL.Path, rule.Request.Path+"/")
	case "regex":
		matched, _ := regexp.MatchString(rule.Request.Path, r.URL.Path)
		return matched
	}

	return false
}
