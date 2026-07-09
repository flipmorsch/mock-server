package server

import (
	"sync"
	"sync/atomic"

	"github.com/flipmorsch/mock-server/internal/rule"
)

// sequences tracks each sequenced rule's position in its ordered responses list.
// State lives here, outside the rule set, so matching stays stateless and an
// atomic reload (which swaps rules) carries a rule's position forward by id.
// Keyed by rule id; only sequenced rules get an entry. See ADR-0007.
type sequences struct {
	mu  sync.RWMutex
	idx map[string]*atomic.Int64
}

func newSequences() *sequences {
	return &sequences{idx: make(map[string]*atomic.Int64)}
}

// seed (re)builds the counter set from the current rules. A position for an id
// that persists is carried over (an unrelated reload must not rewind a client
// mid-poll); a new sequenced rule starts at zero; a vanished id drops. Called
// under the server lock on load, reload, and save.
func (s *sequences) seed(rules []rule.Rule) {
	next := make(map[string]*atomic.Int64)
	s.mu.Lock()
	for i := range rules {
		if !rules[i].Sequenced() {
			continue
		}
		if c, ok := s.idx[rules[i].ID]; ok {
			next[rules[i].ID] = c // preserve position across the swap
		} else {
			next[rules[i].ID] = new(atomic.Int64)
		}
	}
	s.idx = next
	s.mu.Unlock()
}

// selectIndex returns the 0-based element to serve for the next match of rule
// id, clamped to n-1 ("last one sticks"). atomic.Add gives concurrent hits
// distinct increasing positions — no duplicate, no skip; running past n is
// harmless since the read clamps. On a miss (a reload race dropped the id) it
// lazily creates the counter so sequencing still holds.
func (s *sequences) selectIndex(id string, n int) int {
	s.mu.RLock()
	c := s.idx[id]
	s.mu.RUnlock()
	if c == nil {
		s.mu.Lock()
		if c = s.idx[id]; c == nil {
			c = new(atomic.Int64)
			s.idx[id] = c
		}
		s.mu.Unlock()
	}
	if i := c.Add(1) - 1; i < int64(n) {
		return int(i)
	}
	return n - 1
}

// reset rewinds every sequence to its first element (test isolation).
func (s *sequences) reset() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.idx {
		c.Store(0)
	}
}
