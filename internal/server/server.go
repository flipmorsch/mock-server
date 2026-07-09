package server

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/flipmorsch/mock-server/internal/rule"
)

type Server struct {
	config      *rule.Config
	workingCopy *rule.Config
	configPath  string
	journal     *Journal
	uiEnabled   bool
	tlsEnabled  bool        // set once at startup before serving; read by the probe
	logger      *log.Logger // nil = silent (embedded/library default)
	unsaved     bool
	mu          sync.RWMutex
	seq         *sequences // per-rule position for sequenced responses
}

func NewServer(cfg *rule.Config, configPath string, journal *Journal, uiEnabled bool) *Server {
	// IDs must exist on the serving config too: Match Explanations carry
	// them for jump-to-rule links.
	for i := range cfg.Rules {
		if cfg.Rules[i].ID == "" {
			cfg.Rules[i].ID = newID()
		}
	}
	seq := newSequences()
	seq.seed(cfg.Rules)
	return &Server{
		config:      cfg,
		workingCopy: cloneConfig(cfg),
		configPath:  configPath,
		journal:     journal,
		uiEnabled:   uiEnabled,
		seq:         seq,
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
		for j := range clone.Rules[i].Responses {
			if d := clone.Rules[i].Responses[j].Delay; d != "" {
				clone.Rules[i].Responses[j].DelayDuration, _ = time.ParseDuration(d)
			}
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

// CreateRule mints a stable id, validates the rule with that id in place, then
// appends it. Validating after minting is what lets a sequenced rule pass the
// "responses requires an explicit id" guard (ADR-0008).
func (s *Server) CreateRule(r rule.Rule) (rule.Rule, error) {
	r.ID = newID()
	if err := rule.CheckRule(r); err != nil {
		return rule.Rule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workingCopy.Rules = append(s.workingCopy.Rules, r)
	s.unsaved = true
	return r, nil
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
	return s.saveLocked()
}

// SaveConfig replaces the working copy with cfg and persists it. The authoring
// island (ADR-0010) owns the working copy client-side and POSTs the whole thing
// on save; the write path (Check → Validate → write → swap serving config) is
// shared with Save. Client-supplied rules already carry ids, so Check's pre-mint
// id-less-sequenced guard sees them and never false-rejects.
func (s *Server) SaveConfig(cfg rule.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.workingCopy
	s.workingCopy = &cfg
	if err := s.saveLocked(); err != nil {
		s.workingCopy = prev // a rejected save must not leave the invalid copy staged
		return err
	}
	return nil
}

// saveLocked persists the current working copy to disk and swaps it into the
// serving config. The caller must hold s.mu.
func (s *Server) saveLocked() error {
	// Validate the working copy BEFORE cloneConfig mints ids, so the
	// "responses requires an explicit id" guard sees the pre-mint state and an
	// id-less sequenced rule can't slip through by getting a minted id first
	// (closes the ADR-0007 landmine). Check is non-mutating; every working-copy
	// rule already carries an id, so this never false-rejects.
	if err := s.workingCopy.Check(); err != nil {
		return err
	}

	// The file keeps the user's body_file references and raw delay strings;
	// body_file content is read at serve time, delays are parsed by Validate.
	// cloneConfig mints IDs for id-less rules, so the id-less-sequenced guard is
	// enforced by the Check above (pre-mint), not here.
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
	s.seq.seed(serving.Rules)
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
	s.seq.seed(cfg.Rules)
	return len(cfg.Rules), nil
}

// Reset clears the journal and rewinds every response sequence to its first
// element — a clean slate for the next test case. Backs both the library's
// m.Reset() and POST /__admin/reset.
func (s *Server) Reset() {
	s.journal.Clear()
	s.seq.reset()
}

func (s *Server) Journal() *Journal {
	return s.journal
}
func (s *Server) UIEnabled() bool {
	return s.uiEnabled
}

// SetTLSEnabled records whether the server is serving HTTPS. Called once at
// startup before serving begins; the UI's probe reads it to pick the scheme.
func (s *Server) SetTLSEnabled(v bool) {
	s.tlsEnabled = v
}

func (s *Server) TLSEnabled() bool {
	return s.tlsEnabled
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
