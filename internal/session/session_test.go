package session

import (
	"context"
	"errors"
	"testing"

	"github.com/mgoodness/liam/internal/provider"
)

func TestNewAssignsAnID(t *testing.T) {
	s := New()
	if s.ID == "" {
		t.Error("New().ID is empty")
	}
}

func TestRecordAccumulatesCostAndTracksLastContext(t *testing.T) {
	s := New()
	s.Record("openrouter/auto", provider.Usage{InputTokens: 100, CachedInputTokens: 20, CostUSD: 0.01})
	s.Record("anthropic/claude-3.7-sonnet", provider.Usage{InputTokens: 200, CachedInputTokens: 0, CostUSD: 0.02})

	if got, want := s.CostUSD, 0.03; got != want {
		t.Errorf("CostUSD = %v, want %v", got, want)
	}
	if got, want := s.Cost(), 0.03; got != want {
		t.Errorf("Cost() = %v, want %v", got, want)
	}
	if got, want := s.LastContextTokens, 200; got != want {
		t.Errorf("LastContextTokens = %d, want %d (most recent turn only)", got, want)
	}
	if got, want := s.LastModel, "anthropic/claude-3.7-sonnet"; got != want {
		t.Errorf("LastModel = %q, want %q (most recent turn only)", got, want)
	}
}

func TestClearResetsStateAndAssignsAFreshID(t *testing.T) {
	s := New()
	oldID := s.ID
	s.Messages = []provider.Message{{Role: "user", Content: "hi"}}
	s.Record("openrouter/auto", provider.Usage{InputTokens: 100, CostUSD: 1.23})

	s.Clear()

	if s.ID == oldID {
		t.Error("Clear() did not assign a fresh ID")
	}
	if s.Messages != nil {
		t.Errorf("Messages = %+v, want nil after Clear()", s.Messages)
	}
	if s.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0 after Clear()", s.CostUSD)
	}
	if s.LastContextTokens != 0 {
		t.Errorf("LastContextTokens = %d, want 0 after Clear()", s.LastContextTokens)
	}
	if s.LastModel != "" {
		t.Errorf("LastModel = %q, want %q after Clear()", s.LastModel, "")
	}
}

// fakeLookup is a ContextLookup that returns a canned max-context-length
// per model id, with no real API calls, plus an optional error and a call
// count so tests can assert caching behavior is the caller's job (the
// concrete openrouter.Provider is what actually caches — this fake just
// records how many times it was asked).
type fakeLookup struct {
	maxByModel map[string]int
	err        error
	calls      int
}

func (f *fakeLookup) MaxContextLength(_ context.Context, model string) (int, error) {
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	return f.maxByModel[model], nil
}

func TestContextPercentComputesFromLastTurnOnly(t *testing.T) {
	s := New()
	s.Record("openrouter/auto", provider.Usage{InputTokens: 1000, CachedInputTokens: 0})
	s.Record("anthropic/claude-3.7-sonnet", provider.Usage{InputTokens: 5000, CachedInputTokens: 5000})

	lookup := &fakeLookup{maxByModel: map[string]int{
		"openrouter/auto":             1000,
		"anthropic/claude-3.7-sonnet": 100000,
	}}

	got, err := s.ContextPercent(context.Background(), lookup)
	if err != nil {
		t.Fatalf("ContextPercent() error = %v", err)
	}
	if want := 0.1; got != want {
		t.Errorf("ContextPercent() = %v, want %v (10000/100000, most recent turn only)", got, want)
	}
	if lookup.calls != 1 {
		t.Errorf("lookup was called %d times, want 1 (looked up LastModel only)", lookup.calls)
	}
}

func TestContextPercentBeforeAnyTurnIsZeroAndSkipsLookup(t *testing.T) {
	s := New()
	lookup := &fakeLookup{maxByModel: map[string]int{"openrouter/auto": 1000}}

	got, err := s.ContextPercent(context.Background(), lookup)
	if err != nil {
		t.Fatalf("ContextPercent() error = %v", err)
	}
	if got != 0 {
		t.Errorf("ContextPercent() = %v, want 0 before any Record", got)
	}
	if lookup.calls != 0 {
		t.Errorf("lookup was called %d times, want 0 (no LastModel to resolve)", lookup.calls)
	}
}

func TestContextPercentPropagatesLookupError(t *testing.T) {
	s := New()
	s.Record("openrouter/auto", provider.Usage{InputTokens: 100})
	lookup := &fakeLookup{err: errors.New("openrouter: rate limited")}

	_, err := s.ContextPercent(context.Background(), lookup)
	if err == nil {
		t.Fatal("ContextPercent() error = nil, want the lookup's error")
	}
}

func TestContextPercentErrorsOnNonPositiveMaxContextLength(t *testing.T) {
	s := New()
	s.Record("openrouter/auto", provider.Usage{InputTokens: 100})
	lookup := &fakeLookup{maxByModel: map[string]int{"openrouter/auto": 0}}

	_, err := s.ContextPercent(context.Background(), lookup)
	if err == nil {
		t.Fatal("ContextPercent() error = nil, want an error for a non-positive max context length")
	}
}
