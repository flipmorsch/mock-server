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
	config     *rule.Config
	configPath string
	journal    *Journal
	uiEnabled  bool
	tlsEnabled bool        // set once at startup before serving; read by the probe
	logger     *log.Logger // nil = silent (embedded/library default)
	mu         sync.RWMutex
	seq        *sequences // per-rule position for sequenced responses
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
		config:     cfg,
		configPath: configPath,
		journal:    journal,
		uiEnabled:  uiEnabled,
		seq:        seq,
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

// Config returns the committed (serving) rule set — the seed the authoring
// island loads (ADR-0010).
func (s *Server) Config() rule.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.config
}

func (s *Server) ListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.ListenAddr()
}

// SaveConfig persists cfg — the whole client-owned working copy (ADR-0010) — to
// disk and swaps it into the serving set. Check runs BEFORE cloneConfig mints
// ids, so the "responses requires an explicit id" guard (ADR-0007) sees the
// pre-mint state; the file keeps the user's body_file refs and raw delay
// strings. A rejected save leaves the serving config untouched.
func (s *Server) SaveConfig(cfg rule.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := cfg.Check(); err != nil {
		return err
	}
	serving := cloneConfig(&cfg)
	if err := serving.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}
	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	s.config = serving
	s.seq.seed(serving.Rules)
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
