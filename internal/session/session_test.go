package session

import (
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
	s.Record(provider.Usage{InputTokens: 100, CachedInputTokens: 20, CostUSD: 0.01})
	s.Record(provider.Usage{InputTokens: 200, CachedInputTokens: 0, CostUSD: 0.02})

	if got, want := s.CostUSD, 0.03; got != want {
		t.Errorf("CostUSD = %v, want %v", got, want)
	}
	if got, want := s.LastContextTokens, 200; got != want {
		t.Errorf("LastContextTokens = %d, want %d (most recent turn only)", got, want)
	}
}

func TestClearResetsStateAndAssignsAFreshID(t *testing.T) {
	s := New()
	oldID := s.ID
	s.Messages = []provider.Message{{Role: "user", Content: "hi"}}
	s.Record(provider.Usage{InputTokens: 100, CostUSD: 1.23})

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
}
