package rule

import (
	"net/http"
	"time"
)

type Rule struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name,omitempty"`
	Request  Request  `yaml:"request"`
	Response Response `yaml:"response,omitempty"`
	// Responses, when non-empty, is an ordered list served one-per-match (Nth
	// match → Nth element, last sticks). Mutually exclusive with Response.
	Responses []Response `yaml:"responses,omitempty"`
}

// Sequenced reports whether the rule serves an ordered response list.
func (r *Rule) Sequenced() bool { return len(r.Responses) > 0 }

type Request struct {
	Method   string            `yaml:"method,omitempty"`
	Path     string            `yaml:"path,omitempty"`
	PathMode string            `yaml:"path_mode,omitempty"`
	Headers  map[string]string `yaml:"headers,omitempty"`
	Query    map[string]string `yaml:"query,omitempty"`
	Body     *BodyMatch        `yaml:"body,omitempty"`
}

type BodyMatch struct {
	Mode  string `yaml:"mode,omitempty"`
	Value string `yaml:"value"`
}

type Response struct {
	Status  int               `yaml:"status" json:"status"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body    string            `yaml:"body,omitempty" json:"body,omitempty"`
	// json tag is explicit: encoding/json's case-fold does NOT bridge the
	// underscore, so a `body_file` key from the UI's responses JSON would
	// silently drop without it (see ADR-0008).
	BodyFile string `yaml:"body_file,omitempty" json:"body_file,omitempty"`
	Delay    string `yaml:"delay,omitempty" json:"delay,omitempty"`
	Template bool   `yaml:"template,omitempty" json:"template,omitempty"`
	// Derived from Delay by Validate on load and on Save's serving clone;
	// never serialized (json:"-" blocks a spoofed value from a direct POST).
	DelayDuration time.Duration `yaml:"-" json:"-"`
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
