package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type Server struct {
	config      *Config
	workingCopy *Config
	configPath  string
	fixturesDir string
	journal     *Journal
	uiEnabled   bool
	unsaved     bool
	mu          sync.RWMutex
}

func newServer(cfg *Config, configPath string, journal *Journal, uiEnabled bool, fixturesDir string) *Server {
	return &Server{
		config:      cfg,
		workingCopy: cloneConfig(cfg),
		configPath:  configPath,
		fixturesDir: fixturesDir,
		journal:     journal,
		uiEnabled:   uiEnabled,
	}
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return &Config{}
	}
	rules := make([]Rule, len(cfg.Rules))
	for i, r := range cfg.Rules {
		rules[i] = cloneRule(r)
		if rules[i].ID == "" {
			rules[i].ID = newUUID()
		}
	}
	return &Config{
		Listen: cfg.Listen,
		Rules:  rules,
	}
}

func cloneRule(r Rule) Rule {
	headers := make(map[string]string)
	for k, v := range r.Request.Headers {
		headers[k] = v
	}
	query := make(map[string]string)
	for k, v := range r.Request.Query {
		query[k] = v
	}
	respHeaders := make(map[string]string)
	for k, v := range r.Response.Headers {
		respHeaders[k] = v
	}

	clone := Rule{
		ID:   r.ID,
		Name: r.Name,
		Request: Request{
			Method:   r.Request.Method,
			Path:     r.Request.Path,
			PathMode: r.Request.PathMode,
			Headers:  headers,
			Query:    query,
		},
		Response: Response{
			Status:        r.Response.Status,
			Headers:       respHeaders,
			Body:          r.Response.Body,
			BodyFile:      r.Response.BodyFile,
			Delay:         r.Response.Delay,
			Template:      r.Response.Template,
			delayDuration: r.Response.delayDuration,
		},
	}
	if r.Request.Body != nil {
		clone.Request.Body = &BodyMatch{Mode: r.Request.Body.Mode, Value: r.Request.Body.Value}
	}
	return clone
}

func (s *Server) Rules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Rules
}

func (s *Server) WorkingCopy() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workingCopy.Rules
}

func (s *Server) ListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.listenAddr()
}

func (s *Server) FindRule(id string) *Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.workingCopy.Rules {
		if s.workingCopy.Rules[i].ID == id {
			return &s.workingCopy.Rules[i]
		}
	}
	return nil
}

func (s *Server) CreateRule(rule Rule) Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule.ID = newUUID()
	s.workingCopy.Rules = append(s.workingCopy.Rules, rule)
	s.unsaved = true
	return rule
}

func (s *Server) UpdateRule(id string, updated Rule) (*Rule, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.workingCopy.Rules {
		if s.workingCopy.Rules[i].ID == id {
			updated.ID = id
			s.workingCopy.Rules[i] = updated
			s.unsaved = true
			return &s.workingCopy.Rules[i], true
		}
	}
	return nil, false
}

func (s *Server) DeleteRule(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.workingCopy.Rules {
		if s.workingCopy.Rules[i].ID == id {
			s.workingCopy.Rules = append(s.workingCopy.Rules[:i], s.workingCopy.Rules[i+1:]...)
			s.unsaved = true
			return true
		}
	}
	return false
}

func (s *Server) ReorderRules(ids []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) != len(s.workingCopy.Rules) {
		return false
	}
	lookup := make(map[string]Rule)
	for _, r := range s.workingCopy.Rules {
		lookup[r.ID] = r
	}
	ordered := make([]Rule, 0, len(ids))
	for _, id := range ids {
		r, ok := lookup[id]
		if !ok {
			return false
		}
		ordered = append(ordered, r)
	}
	s.workingCopy.Rules = ordered
	s.unsaved = true
	return true
}

func (s *Server) UpdateConfig(listen string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workingCopy.Listen = listen
	s.unsaved = true
}

func (s *Server) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.workingCopy
}

func (s *Server) HasUnsaved() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unsaved
}

func (s *Server) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := yaml.Marshal(s.workingCopy)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	s.config = cloneConfig(s.workingCopy)
	s.unsaved = false
	return nil
}

func (s *Server) ResolveFixturePath(filename string) string {
	return filepath.Join(s.fixturesDir, filepath.Base(filename))
}

func (s *Server) matchRule(r *http.Request, body []byte) (*Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.config.Rules {
		if match(&s.config.Rules[i], r, body) {
			return &s.config.Rules[i], true
		}
	}
	return nil, false
}
