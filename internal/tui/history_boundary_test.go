package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
)

// TestUpRecallsHistoryWhenInputEmpty and its siblings below cover issue
// #58's Up/Down boundary logic directly through Model.Update, exercising
// the exact "empty input" / "cursor on the textarea's first line" /
// "cursor on the textarea's last line" conditions the spec calls out,
// versus ordinary multi-line cursor movement everywhere else.

func TestUpRecallsHistoryWhenInputEmpty(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("first")
	m.hist.add("second")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	mm := next.(Model)

	if got := mm.input.Value(); got != "second" {
		t.Errorf("input.Value() = %q, want %q (the newest history entry)", got, "second")
	}
}

func TestDownRecallsHistoryWhenInputEmpty(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("first")
	m.hist.add("second")
	// Start cycling via Up so Down has somewhere newer to return to.
	m.hist.up("")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mm := next.(Model)

	if got := mm.input.Value(); got != "" {
		t.Errorf("input.Value() = %q, want %q (the preserved empty draft)", got, "")
	}
}

func TestUpOnSingleLineInputRecallsHistory(t *testing.T) {
	// A single-line, non-empty input is trivially "on the first line" —
	// Up must still recall rather than move the cursor within the line.
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("recalled message")
	m.input.SetValue("typing something")
	m.input.CursorEnd()

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	mm := next.(Model)

	if got := mm.input.Value(); got != "recalled message" {
		t.Errorf("input.Value() = %q, want %q (history recalled)", got, "recalled message")
	}
}

func TestUpOnNonFirstLineOfMultilineInputMovesCursorNotHistory(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("should not appear")
	m.input.SetValue("line1\nline2\nline3")
	m.input.CursorUp() // row 2 -> row 1 (not the first line)
	if m.input.Line() != 1 {
		t.Fatalf("setup: input.Line() = %d, want 1", m.input.Line())
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	mm := next.(Model)

	if got := mm.input.Value(); got != "line1\nline2\nline3" {
		t.Errorf("input.Value() = %q, want the multi-line draft untouched (no history recall)", got)
	}
	if mm.input.Line() != 0 {
		t.Errorf("input.Line() = %d, want 0 (cursor moved up normally)", mm.input.Line())
	}
}

func TestUpOnFirstLineOfMultilineInputRecallsHistory(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("recalled message")
	m.input.SetValue("line1\nline2\nline3")
	m.input.CursorUp()
	m.input.CursorUp() // row 2 -> row 0 (the first line)
	if m.input.Line() != 0 {
		t.Fatalf("setup: input.Line() = %d, want 0", m.input.Line())
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	mm := next.(Model)

	if got := mm.input.Value(); got != "recalled message" {
		t.Errorf("input.Value() = %q, want %q (history recalled from the first line)", got, "recalled message")
	}
}

func TestDownOnNonLastLineOfMultilineInputMovesCursorNotHistory(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("should not appear")
	m.hist.up("") // start cycling, so a wrongly-triggered recall would be observable
	m.input.SetValue("line1\nline2\nline3")
	m.input.CursorUp() // row 2 -> row 1 (not the last line)
	if m.input.Line() != 1 {
		t.Fatalf("setup: input.Line() = %d, want 1", m.input.Line())
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mm := next.(Model)

	if got := mm.input.Value(); got != "line1\nline2\nline3" {
		t.Errorf("input.Value() = %q, want the multi-line draft untouched (no history recall)", got)
	}
	if mm.input.Line() != 2 {
		t.Errorf("input.Line() = %d, want 2 (cursor moved down normally)", mm.input.Line())
	}
}

func TestDownOnLastLineOfMultilineInputRecallsHistory(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("first")
	m.hist.add("second")
	m.hist.up("draft") // idx now points at "second"; Down should move to the draft
	m.input.SetValue("line1\nline2\nline3")
	if m.input.Line() != 2 {
		t.Fatalf("setup: input.Line() = %d, want 2 (SetValue leaves the cursor at the end)", m.input.Line())
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mm := next.(Model)

	if got := mm.input.Value(); got != "draft" {
		t.Errorf("input.Value() = %q, want %q (the preserved draft, recalled from the last line)", got, "draft")
	}
}
