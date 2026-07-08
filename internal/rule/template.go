package rule

import (
	"math/rand/v2"
	"net/http"
	"strings"
	"sync/atomic"
	"text/template"
	"time"
)

var globalCounter atomic.Int64

func templateFuncs(counter func(*RequestFilter) int64) template.FuncMap {
	return template.FuncMap{
		"now": func() string {
			return time.Now().Format(time.RFC3339)
		},
		"nowFormat": func(layout string) string {
			return time.Now().Format(layout)
		},
		"randomInt": func(min, max int) int {
			return rand.IntN(max-min+1) + min
		},
		"randomString": func(n int) string {
			const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			var sb strings.Builder
			sb.Grow(n)
			for i := 0; i < n; i++ {
				sb.WriteByte(letters[rand.IntN(len(letters))])
			}
			return sb.String()
		},
		"counter": func() int64 {
			return globalCounter.Add(1)
		},
		"requestCount": func(args ...string) int64 {
			f := &RequestFilter{}
			if len(args) >= 1 {
				f.Method = args[0]
			}
			if len(args) >= 2 {
				f.Path = args[1]
			}
			return counter(f)
		},
	}
}

func ExecuteTemplate(body string, r *http.Request, reqBody []byte, counter func(*RequestFilter) int64) (string, error) {
	tmpl, err := template.New("response").Funcs(templateFuncs(counter)).Parse(body)
	if err != nil {
		return "", err
	}

	data := &templateData{
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
