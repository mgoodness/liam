package tui

import (
	"testing"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
)

// TestStreamingSurvivesModelCopiedByValue is a regression test for a live
// crash: "strings: illegal use of non-zero Builder copied by value",
// panicking inside handleEvent's m.streaming.WriteString(e.Text) after a
// find/grep tool call (issue found via /diagnosing-bugs, reported as
// "liam: find/grep searcher=fff-mcp").
//
// Root cause: Model is copied by value on every single Update/View call —
// Bubbletea's Elm architecture returns/reassigns tea.Model by value, and
// every value receiver method on Model (View, submit, handleUp, etc.)
// takes its own copy — and strings.Builder is documented as unsafe to
// store as a value field in anything that gets copied after a write ("Do
// not copy a non-zero Builder"; see strings.Builder.copyCheck). A plain
// single-goroutine drive of the concrete Model type doesn't reproduce this
// (escape analysis happens to keep the address stable in that shape), and
// neither did a real tea.Program run in this package's own experiments —
// the panic is inherently dependent on exactly when/where the Go runtime
// physically relocates the struct, which varies with allocator and
// goroutine-scheduling behavior outside a test's control. So instead of
// chasing that non-determinism, this test forces the exact hazard
// strings.Builder's docs describe directly: copy a Model whose streaming
// buffer has already been written to into fresh memory (a slice element,
// guaranteed distinct from the source and from each other), then write to
// it again. This deterministically panicked before streaming became a
// *strings.Builder (a copy of Model then only copies the pointer, so every
// copy still shares the one underlying Builder) and cannot panic after.
func TestStreamingSurvivesModelCopiedByValue(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.handleEvent(provider.TextDeltaEvent{Text: "abc"})

	copies := make([]Model, 4)
	for i := range copies {
		copies[i] = m
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("streaming panicked after Model was copied by value: %v", r)
		}
	}()
	copies[len(copies)-1].handleEvent(provider.TextDeltaEvent{Text: "def"})

	if got, want := copies[len(copies)-1].streaming.String(), "abcdef"; got != want {
		t.Errorf("streaming.String() = %q, want %q", got, want)
	}
}
