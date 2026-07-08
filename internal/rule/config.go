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
	case "exact", "prefix", "regex":
	default:
		return fmt.Errorf("unsupported path_mode %q (supported: exact, prefix, regex)", r.Request.PathMode)
	}
	if mode == "regex" {
		if _, err := regexp.Compile(r.Request.Path); err != nil {
			return fmt.Errorf("invalid regex pattern %q: %w", r.Request.Path, err)
		}
	}
	if r.Request.Body != nil {
		switch r.Request.Body.Mode {
		case "exact", "contains", "":
		default:
			return fmt.Errorf("unsupported body mode %q (supported: exact, contains)", r.Request.Body.Mode)
		}
	}
	if r.Response.Body != "" && r.Response.BodyFile != "" {
		return fmt.Errorf("body and body_file are mutually exclusive")
	}
	if r.Response.Delay != "" {
		if _, err := time.ParseDuration(r.Response.Delay); err != nil {
			return fmt.Errorf("invalid delay %q: %w", r.Response.Delay, err)
		}
	}
	if r.Response.Status != 0 && (r.Response.Status < 100 || r.Response.Status > 599) {
		return fmt.Errorf("status %d out of range (100-599)", r.Response.Status)
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
	for i, rule := range c.Rules {
		if rule.Response.Headers != nil {
			canonical := make(map[string]string, len(rule.Response.Headers))
			for k, v := range rule.Response.Headers {
				canonical[http.CanonicalHeaderKey(k)] = v
			}
			rule.Response.Headers = canonical
		}
		if rule.Request.Headers != nil {
			canonical := make(map[string]string, len(rule.Request.Headers))
			for k, v := range rule.Request.Headers {
				canonical[http.CanonicalHeaderKey(k)] = v
			}
			rule.Request.Headers = canonical
		}
		if rule.Request.Body != nil && rule.Request.Body.Mode == "" {
			rule.Request.Body.Mode = "exact"
		}
		if rule.Response.Status == 0 {
			rule.Response.Status = 200
		}
		if rule.Response.BodyFile != "" {
			// Readability check only — the file is read at serve time, so the
			// config keeps the user's reference and fixture edits apply live.
			if _, err := os.ReadFile(rule.Response.BodyFile); err != nil {
				return fmt.Errorf("rule %d (%q): reading body_file %q: %w", i+1, rule.Name, rule.Response.BodyFile, err)
			}
		}
		if rule.Response.Delay != "" {
			rule.Response.DelayDuration, _ = time.ParseDuration(rule.Response.Delay)
		}
		c.Rules[i] = rule
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
