package main

import (
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"text/template"
	"time"
)

var globalCounter atomic.Int64

type templateData struct {
	Method string
	Path   string
	Body   string
	req    *http.Request
}

func (d templateData) Header(name string) string {
	return d.req.Header.Get(name)
}

func (d templateData) Query(name string) string {
	return d.req.URL.Query().Get(name)
}

var templateFuncs = template.FuncMap{
	"now": func() string {
		return time.Now().Format(time.RFC3339)
	},
	"nowFormat": func(layout string) string {
		return time.Now().Format(layout)
	},
	"randomInt": func(min, max int) int {
		return rand.Intn(max-min+1) + min
	},
	"randomString": func(n int) string {
		const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		var sb strings.Builder
		sb.Grow(n)
		for i := 0; i < n; i++ {
			sb.WriteByte(letters[rand.Intn(len(letters))])
		}
		return sb.String()
	},
	"counter": func() int64 {
		return globalCounter.Add(1)
	},
}

func executeTemplate(body string, r *http.Request, reqBody []byte) (string, error) {
	tmpl, err := template.New("response").Funcs(templateFuncs).Parse(body)
	if err != nil {
		return "", err
	}

	data := templateData{
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   string(reqBody),
		req:    r,
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}
