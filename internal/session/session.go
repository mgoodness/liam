// Package session holds one continuous conversation's mutable state: its
// id, message history, and the lightweight context/cost tracker /clear
// resets alongside them — the tracker described in issue #52, feeding the
// future statusLine ticket (#19).
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/mgoodness/liam/internal/provider"
)

// ContextLookup resolves model's maximum context length in tokens, e.g.
// via OpenRouter's /models endpoint. Implementations are expected to
// cache per model id, since e.g. "openrouter/auto" can route to a
// different underlying model turn to turn — narrow enough for tests to
// substitute a fake without hitting a real API.
type ContextLookup interface {
	MaxContextLength(ctx context.Context, model string) (int, error)
}

// Session is one session's history plus its running cost/context tallies.
type Session struct {
	ID       string
	Messages []provider.Message

	// CostUSD is a running sum of Usage.CostUSD across every turn.
	CostUSD float64
	// LastContextTokens is InputTokens+CachedInputTokens from the most
	// recent turn's Usage.
	LastContextTokens int
	// LastModel is the ModelUsed from the most recent turn's DoneEvent,
	// resolved against a ContextLookup by ContextPercent.
	LastModel string
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
	s.LastModel = ""
}

// Record folds one turn's Usage into the running cost/context tallies, and
// notes model (a DoneEvent's ModelUsed) for the next ContextPercent call.
func (s *Session) Record(model string, u provider.Usage) {
	s.CostUSD += u.CostUSD
	s.LastContextTokens = u.InputTokens + u.CachedInputTokens
	s.LastModel = model
}

// Cost returns the running-sum cost across the session, for the future
// statusLine (#19) to consume.
func (s *Session) Cost() float64 { return s.CostUSD }

// ContextPercent returns the most recent turn's context usage as a
// fraction of LastModel's max context length (0 before any turn has been
// recorded), resolving that length via lookup.
func (s *Session) ContextPercent(ctx context.Context, lookup ContextLookup) (float64, error) {
	if s.LastModel == "" {
		return 0, nil
	}
	maxLen, err := lookup.MaxContextLength(ctx, s.LastModel)
	if err != nil {
		return 0, err
	}
	if maxLen <= 0 {
		return 0, fmt.Errorf("session: model %q reported non-positive max context length", s.LastModel)
	}
	return float64(s.LastContextTokens) / float64(maxLen), nil
}

// ResetContext clears LastContextTokens and LastModel (leaving CostUSD
// untouched), so ContextPercent reports 0 again until the next Record call
// repopulates them. Compaction (issue #54) calls this whenever it condenses
// the conversation, matching the spec'd statusLine behavior: the reported
// context percentage goes back to unset immediately after compaction
// rather than liam estimating the freshly-summarized size itself.
func (s *Session) ResetContext() {
	s.LastContextTokens = 0
	s.LastModel = ""
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
