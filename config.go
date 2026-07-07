package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string `yaml:"listen"`
	Rules  []Rule `yaml:"rules"`
}

type Rule struct {
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
	Status   int               `yaml:"status"`
	Headers  map[string]string `yaml:"headers"`
	Body     string            `yaml:"body"`
	BodyFile string            `yaml:"body_file"`
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

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	for i, rule := range c.Rules {
		if rule.Request.Method == "" {
			return fmt.Errorf("rule %d (%q): method is required", i+1, rule.Name)
		}
		if rule.Request.Path == "" {
			return fmt.Errorf("rule %d (%q): path is required", i+1, rule.Name)
		}
		mode := rule.Request.PathMode
		if mode == "" {
			mode = "exact"
		}
		switch mode {
		case "exact", "prefix", "regex":
		default:
			return fmt.Errorf("rule %d (%q): unsupported path_mode %q (supported: exact, prefix, regex)", i+1, rule.Name, rule.Request.PathMode)
		}
		if mode == "regex" {
			_, err := regexp.Compile(rule.Request.Path)
			if err != nil {
				return fmt.Errorf("rule %d (%q): invalid regex pattern %q: %w", i+1, rule.Name, rule.Request.Path, err)
			}
		}
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
		if rule.Request.Body != nil {
			switch rule.Request.Body.Mode {
			case "exact", "contains":
			case "":
				rule.Request.Body.Mode = "exact"
			default:
				return fmt.Errorf("rule %d (%q): unsupported body mode %q (supported: exact, contains)", i+1, rule.Name, rule.Request.Body.Mode)
			}
		}
		if rule.Response.Body != "" && rule.Response.BodyFile != "" {
			return fmt.Errorf("rule %d (%q): body and body_file are mutually exclusive", i+1, rule.Name)
		}
		if rule.Response.BodyFile != "" {
			bodyData, err := os.ReadFile(rule.Response.BodyFile)
			if err != nil {
				return fmt.Errorf("rule %d (%q): reading body_file %q: %w", i+1, rule.Name, rule.Response.BodyFile, err)
			}
			rule.Response.Body = string(bodyData)
		}
		rule.Response.BodyFile = ""
		c.Rules[i] = rule
	}

	return nil
}

func (c *Config) listenAddr() string {
	if c.Listen != "" {
		return c.Listen
	}
	return "127.0.0.1:8080"
}

func normalizeMethod(m string) string {
	return strings.ToUpper(strings.TrimSpace(m))
}
