package rule

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string `yaml:"listen"`
	Rules  []Rule `yaml:"rules"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return ParseConfig(data)
}

// ParseConfig parses and validates a YAML config from bytes (used by the
// embeddable library, which has no file on disk).
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadOrEmpty(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Config{}, nil
	}
	return LoadConfig(path)
}

// CheckRule validates a single rule without mutating it. Used per-rule by
// the UI on create/update and by Check before a save is written to disk.
func CheckRule(r Rule) error {
	if r.Request.Method == "" {
		return fmt.Errorf("method is required")
	}
	if r.Request.Path == "" {
		return fmt.Errorf("path is required")
	}
	mode := r.Request.PathMode
	if mode == "" {
		mode = "exact"
	}
	switch mode {
	case "exact", "prefix", "regex", "pattern":
	default:
		return fmt.Errorf("unsupported path_mode %q (supported: exact, prefix, regex, pattern)", r.Request.PathMode)
	}
	if mode == "regex" {
		if _, err := regexp.Compile(r.Request.Path); err != nil {
			return fmt.Errorf("invalid regex pattern %q: %w", r.Request.Path, err)
		}
	}
	if mode == "pattern" {
		if err := checkPattern(r.Request.Path); err != nil {
			return fmt.Errorf("invalid path pattern %q: %w", r.Request.Path, err)
		}
	}
	if r.Request.Body != nil {
		switch r.Request.Body.Mode {
		case "exact", "contains", "json", "":
		default:
			return fmt.Errorf("unsupported body mode %q (supported: exact, contains, json)", r.Request.Body.Mode)
		}
	}
	if r.Responses != nil && len(r.Responses) == 0 {
		return fmt.Errorf("responses must have at least one element")
	}
	if r.Sequenced() {
		if responseIsSet(r.Response) {
			return fmt.Errorf("response and responses are mutually exclusive")
		}
		if r.ID == "" {
			return fmt.Errorf("responses requires an explicit id (sequence state is keyed by id and would reset on reload without one)")
		}
		for i := range r.Responses {
			if err := checkResponse(r.Responses[i]); err != nil {
				return fmt.Errorf("responses[%d]: %w", i, err)
			}
		}
		return nil
	}
	return checkResponse(r.Response)
}

var paramNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// checkPattern verifies a "pattern" path's {name} placeholders are well-formed:
// every '{' opens a valid, unique {name} group and there are no stray braces. A
// malformed brace would otherwise be escaped into a literal and silently never
// match; a duplicate name would silently collapse to one param when captured.
func checkPattern(p string) error {
	seen := map[string]bool{}
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '{':
			end := strings.IndexByte(p[i:], '}')
			if end < 0 {
				return fmt.Errorf("unclosed '{'")
			}
			name := p[i+1 : i+end]
			if !paramNameRe.MatchString(name) {
				return fmt.Errorf("invalid parameter name %q (letters, digits, underscore; must not start with a digit)", name)
			}
			if seen[name] {
				return fmt.Errorf("duplicate parameter name %q", name)
			}
			seen[name] = true
			i += end
		case '}':
			return fmt.Errorf("unexpected '}'")
		}
	}
	return nil
}

// responseIsSet reports whether any response field was populated. Used to reject
// a rule that sets both singular response and the responses list.
func responseIsSet(r Response) bool {
	return r.Status != 0 || r.Body != "" || r.BodyFile != "" ||
		len(r.Headers) > 0 || r.Delay != "" || r.Template
}

// checkResponse validates one response's fields without mutating it.
func checkResponse(r Response) error {
	if r.Body != "" && r.BodyFile != "" {
		return fmt.Errorf("body and body_file are mutually exclusive")
	}
	if r.Delay != "" {
		if _, err := time.ParseDuration(r.Delay); err != nil {
			return fmt.Errorf("invalid delay %q: %w", r.Delay, err)
		}
	}
	if r.Status != 0 && (r.Status < 100 || r.Status > 599) {
		return fmt.Errorf("status %d out of range (100-599)", r.Status)
	}
	return nil
}

// Check validates every rule without mutating the config.
func (c *Config) Check() error {
	for i, r := range c.Rules {
		if err := CheckRule(r); err != nil {
			return fmt.Errorf("rule %d (%q): %w", i+1, r.Name, err)
		}
	}
	return nil
}

// Validate checks the config and normalizes it for serving: canonical header
// keys, default body mode, parsed delay, and body_file inlined into Body.
func (c *Config) Validate() error {
	if err := c.Check(); err != nil {
		return err
	}
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.Request.Headers != nil {
			canonical := make(map[string]string, len(r.Request.Headers))
			for k, v := range r.Request.Headers {
				canonical[http.CanonicalHeaderKey(k)] = v
			}
			r.Request.Headers = canonical
		}
		if r.Request.Body != nil && r.Request.Body.Mode == "" {
			r.Request.Body.Mode = "exact"
		}
		if r.Sequenced() {
			for j := range r.Responses {
				if err := normalizeResponse(&r.Responses[j]); err != nil {
					return fmt.Errorf("rule %d (%q) responses[%d]: %w", i+1, r.Name, j, err)
				}
			}
		} else {
			if err := normalizeResponse(&r.Response); err != nil {
				return fmt.Errorf("rule %d (%q): %w", i+1, r.Name, err)
			}
		}
	}
	return nil
}

// normalizeResponse prepares one response for serving: canonical header keys,
// default status, delay parsed, and body_file readability checked (read at serve
// time, so the reference is kept and fixture edits apply live).
func normalizeResponse(r *Response) error {
	if r.Headers != nil {
		canonical := make(map[string]string, len(r.Headers))
		for k, v := range r.Headers {
			canonical[http.CanonicalHeaderKey(k)] = v
		}
		r.Headers = canonical
	}
	if r.Status == 0 {
		r.Status = 200
	}
	if r.BodyFile != "" {
		if _, err := os.ReadFile(r.BodyFile); err != nil {
			return fmt.Errorf("reading body_file %q: %w", r.BodyFile, err)
		}
	}
	if r.Delay != "" {
		r.DelayDuration, _ = time.ParseDuration(r.Delay)
	}
	return nil
}

func (c *Config) ListenAddr() string {
	if c.Listen != "" {
		return c.Listen
	}
	return "127.0.0.1:8080"
}

func NormalizeMethod(m string) string {
	return strings.ToUpper(strings.TrimSpace(m))
}
