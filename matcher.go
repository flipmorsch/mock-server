package main

import (
	"net/http"
	"regexp"
	"strings"
)

func match(rule *Rule, r *http.Request) bool {
	if normalizeMethod(rule.Request.Method) != normalizeMethod(r.Method) {
		return false
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
