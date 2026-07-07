package main

import "net/http"

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
	}

	return false
}
