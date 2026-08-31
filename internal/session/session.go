// Package session holds one continuous conversation's mutable state: its
// id, message history, and the lightweight context/cost tracker /clear
// resets alongside them. The full context-percentage and cost-tracking
// mechanism (per-model max-context lookups, statusLine wiring) is ticket
// #52's job; this is the minimal seat for it that /clear already needs to
// reset today.
package session

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/mgoodness/liam/internal/provider"
)

// Session is one session's history plus its running cost/context tallies.
type Session struct {
	ID       string
	Messages []provider.Message

	// CostUSD is a running sum of Usage.CostUSD across every turn.
	CostUSD float64
	// LastContextTokens is InputTokens+CachedInputTokens from the most
	// recent turn's Usage.
	LastContextTokens int
}

// New starts a fresh Session with a freshly assigned ID.
func New() *Session {
	return &Session{ID: newID()}
}

// Clear resets messages and the cost/context tracker, and assigns a fresh
// session ID.
func (s *Session) Clear() {
	s.ID = newID()
	s.Messages = nil
	s.CostUSD = 0
	s.LastContextTokens = 0
}

// Record folds one turn's Usage into the running cost/context tallies.
func (s *Session) Record(u provider.Usage) {
	s.CostUSD += u.CostUSD
	s.LastContextTokens = u.InputTokens + u.CachedInputTokens
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
