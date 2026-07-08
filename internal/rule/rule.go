package rule

import (
	"net/http"
	"time"
)

type Rule struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Request  Request  `yaml:"request"`
	Response Response `yaml:"response"`
}

type Request struct {
	Method   string            `yaml:"method"`
	Path     string            `yaml:"path"`
	PathMode string            `yaml:"path_mode"`
	Headers  map[string]string `yaml:"headers"`
	Query    map[string]string `yaml:"query"`
	Body     *BodyMatch        `yaml:"body"`
}

type BodyMatch struct {
	Mode  string `yaml:"mode"`
	Value string `yaml:"value"`
}

type Response struct {
	Status        int               `yaml:"status"`
	Headers       map[string]string `yaml:"headers"`
	Body          string            `yaml:"body"`
	BodyFile      string            `yaml:"body_file"`
	Delay         string            `yaml:"delay"`
	Template      bool              `yaml:"template"`
	DelayDuration time.Duration
}

type RequestFilter struct {
	Method   string
	Path     string
	PathMode string
	Headers  map[string]string
	Query    map[string]string
	BodyMode string
	Body     string
}

type templateData struct {
	Method string
	Path   string
	Body   string
	req    *http.Request
}

func (d *templateData) Header(name string) string {
	return d.req.Header.Get(name)
}

func (d *templateData) Query(name string) string {
	return d.req.URL.Query().Get(name)
}
