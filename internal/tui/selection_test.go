package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
)

// click simulates a left-button mouse-down at (x, y).
func click(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(Model)
}

// drag simulates a left-button-held motion to (x, y).
func drag(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	next, _ := m.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(Model)
}

// release simulates a left-button-up at (x, y), returning both the
// resulting Model and any command Update produced (the OSC-52 clipboard
// write, when the drag copied something).
func release(t *testing.T, m Model, x, y int) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(Model), cmd
}

// clipboardText runs cmd (expected to be the Cmd tea.SetClipboard produced)
// and returns the string it would write via OSC-52. setClipboardMsg is an
// unexported type in charm.land/bubbletea/v2, so %v is the only way to read
// its content from outside the package — verified against the underlying
// string type's default fmt behavior (no custom String method).
func clipboardText(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	if cmd == nil {
		t.Fatal("clipboardText: nil cmd, want tea.SetClipboard's command")
	}
	return fmt.Sprintf("%v", cmd())
}

// TestClickWithoutDragCopiesNothingAndLeavesNoHighlight covers the AC: a
// single-point click with no movement between press and release must not
// copy anything or leave a highlight.
func TestClickWithoutDragCopiesNothingAndLeavesNoHighlight(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	m = click(t, m, 5, 0)
	if !m.sel.dragging {
		t.Fatal("click did not start a drag")
	}

	final, cmd := release(t, m, 5, 0)
	if cmd != nil {
		t.Error("no-movement click produced a clipboard command, want nil")
	}
	if final.sel.hasRange() {
		t.Error("no-movement click left a selection range, want none")
	}
	if strings.Contains(final.viewport.View(), "\x1b[7m") {
		t.Error("no-movement click left a reverse-video highlight on screen")
	}
}

// TestDragSelectsAndCopiesSpannedText covers the core AC: click-drag across
// several transcript rows highlights the span and, on release, copies the
// spanned plain text (line breaks preserved, no ANSI) to the clipboard via
// OSC-52 — with no confirmation step (a bare Update call is all it takes).
func TestDragSelectsAndCopiesSpannedText(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)
	// sized's 30 "line N" rows plus the trailing blank separator row (31
	// content lines total) against an 8-row viewport put the visible rows
	// at content lines 23..30 (YOffset 23): screen row 0 is "line 23".
	m = click(t, m, 0, 0)
	m = drag(t, m, m.width-1, 2)

	if !strings.Contains(m.viewport.View(), "\x1b[7m") {
		t.Error("in-progress drag did not render a reverse-video highlight")
	}

	final, cmd := release(t, m, m.width-1, 2)
	got := clipboardText(t, cmd)
	want := "line 23\nline 24\nline 25"
	if got != want {
		t.Errorf("copied text = %q, want %q", got, want)
	}
	if !final.sel.hasRange() {
		t.Error("selection was cleared on release, want the highlight to persist")
	}
}

// TestDragUpwardNormalizesSelection covers normalized(): dragging from a
// later row to an earlier one must select/copy identically to the same span
// dragged the other way.
func TestDragUpwardNormalizesSelection(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)
	m = click(t, m, m.width-1, 2)
	m = drag(t, m, 0, 0)

	_, cmd := release(t, m, 0, 0)
	got := clipboardText(t, cmd)
	want := "line 23\nline 24\nline 25"
	if got != want {
		t.Errorf("copied text = %q, want %q", got, want)
	}
}

// TestMotionWithoutPriorClickIsNoOp covers the AC scoping selection to
// drags that actually started in the transcript: a motion event with no
// button-down beforehand (e.g. one delivered despite no active drag) must
// not start or extend a selection.
func TestMotionWithoutPriorClickIsNoOp(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	m = drag(t, m, 5, 3)
	if m.sel.hasRange() || m.sel.dragging {
		t.Errorf("motion with no prior click produced a selection: %+v", m.sel)
	}
}

// TestClickBelowTranscriptDoesNotSelect covers the AC that mouse
// interaction starting in the input field or status block (below the
// transcript's own screen rows) must not engage transcript selection.
func TestClickBelowTranscriptDoesNotSelect(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)
	belowTranscript := m.viewport.Height() // first row outside the transcript

	m = click(t, m, 0, belowTranscript)
	if m.sel.dragging {
		t.Fatal("click below the transcript started a drag")
	}

	m = drag(t, m, m.width-1, belowTranscript)
	final, cmd := release(t, m, m.width-1, belowTranscript)
	if cmd != nil {
		t.Error("click-drag-release below the transcript produced a clipboard command")
	}
	if final.sel.hasRange() {
		t.Error("click-drag-release below the transcript left a selection")
	}
}

// TestScrollWheelStillWorksDuringSelection covers the AC that scroll-wheel
// scrolling of the transcript is unaffected by the selection feature —
// unrelated to whether a drag is in progress.
func TestScrollWheelStillWorksDuringSelection(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)
	m = click(t, m, 0, 0)
	m = drag(t, m, m.width-1, 2)

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	mm := next.(Model)
	if mm.viewport.AtBottom() {
		t.Error("mouse wheel up did not scroll the viewport during an in-progress selection")
	}
}

// TestNewClickClearsPriorSelection covers the click-to-deselect behavior:
// starting a fresh click in the transcript clears whatever selection (drag
// in progress or already-released) was there before.
func TestNewClickClearsPriorSelection(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)
	m = click(t, m, 0, 0)
	m = drag(t, m, m.width-1, 2)
	m, _ = release(t, m, m.width-1, 2)
	if !m.sel.hasRange() {
		t.Fatal("setup: expected a persisted selection after release")
	}

	m = click(t, m, 0, 0)
	if m.sel.hasRange() {
		t.Error("a new click did not clear the prior selection")
	}
}
