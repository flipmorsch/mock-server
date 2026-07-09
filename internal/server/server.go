package server

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"mock-server/internal/rule"
)

type Server struct {
	config      *rule.Config
	workingCopy *rule.Config
	configPath  string
	journal     *Journal
	uiEnabled   bool
	unsaved     bool
	mu          sync.RWMutex
}

func NewServer(cfg *rule.Config, configPath string, journal *Journal, uiEnabled bool) *Server {
	// IDs must exist on the serving config too: Match Explanations carry
	// them for jump-to-rule links.
	for i := range cfg.Rules {
		if cfg.Rules[i].ID == "" {
			cfg.Rules[i].ID = newID()
		}
	}
	return &Server{
		config:      cfg,
		workingCopy: cloneConfig(cfg),
		configPath:  configPath,
		journal:     journal,
		uiEnabled:   uiEnabled,
	}
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func cloneConfig(cfg *rule.Config) *rule.Config {
	if cfg == nil {
		return &rule.Config{}
	}
	// ponytail: deep clone via yaml round-trip; config is small and already yaml-serializable
	data, _ := yaml.Marshal(cfg)
	var clone rule.Config
	yaml.Unmarshal(data, &clone)
	for i := range clone.Rules {
		if clone.Rules[i].ID == "" {
			clone.Rules[i].ID = newID()
		}
		// DelayDuration is yaml:"-" and doesn't survive the round-trip.
		if d := clone.Rules[i].Response.Delay; d != "" {
			clone.Rules[i].Response.DelayDuration, _ = time.ParseDuration(d)
		}
	}
	return &clone
}

func (s *Server) WorkingCopy() []rule.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workingCopy.Rules
}

func (s *Server) ListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.ListenAddr()
}

func (s *Server) FindRule(id string) *rule.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.workingCopy.Rules {
		if s.workingCopy.Rules[i].ID == id {
			return &s.workingCopy.Rules[i]
		}
	}
	return nil
}

func (s *Server) CreateRule(r rule.Rule) rule.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = newID()
	s.workingCopy.Rules = append(s.workingCopy.Rules, r)
	s.unsaved = true
	return r
}

func (s *Server) UpdateRule(id string, updated rule.Rule) (*rule.Rule, bool) {
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
	lookup := make(map[string]rule.Rule)
	for _, r := range s.workingCopy.Rules {
		lookup[r.ID] = r
	}
	ordered := make([]rule.Rule, 0, len(ids))
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

func (s *Server) GetConfig() rule.Config {
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

	// The file keeps the user's body_file references and raw delay strings;
	// body_file content is read at serve time, delays are parsed by Validate.
	serving := cloneConfig(s.workingCopy)
	if err := serving.Validate(); err != nil {
		return err
	}

	data, err := yaml.Marshal(s.workingCopy)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	s.config = serving
	s.unsaved = false
	return nil
}

// Reload re-reads the config file from disk, validates it, and atomically
// swaps the serving rule set, returning the number of rules now serving. On
// any error the current rules are left unchanged. Headless-only: it never
// touches the UI's working copy, so it must not run while --ui owns the rules.
func (s *Server) Reload() (int, error) {
	cfg, err := rule.LoadConfig(s.configPath)
	if err != nil {
		return 0, err
	}
	// Reloaded rules need IDs too: /__admin/ and the journal carry them.
	for i := range cfg.Rules {
		if cfg.Rules[i].ID == "" {
			cfg.Rules[i].ID = newID()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
	return len(cfg.Rules), nil
}

func (s *Server) Journal() *Journal {
	return s.journal
}
func (s *Server) UIEnabled() bool {
	return s.uiEnabled
}

// MatchRule walks the serving rules in order. It returns the first match
// (nil if none) plus the verdicts of every rule evaluated and missed along
// the way — the raw material for Match Explanations.
func (s *Server) MatchRule(r *http.Request, body []byte) (*rule.Rule, []rule.RuleVerdict) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var misses []rule.RuleVerdict
	for i := range s.config.Rules {
		rv := rule.Explain(&s.config.Rules[i], r, body)
		if rv.Matched {
			return &s.config.Rules[i], misses
		}
		misses = append(misses, rv)
	}
	return nil, misses
}
