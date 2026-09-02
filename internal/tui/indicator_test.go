package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/theme"
	"github.com/mgoodness/liam/internal/tool"
)

func TestEstimatedOutputTokens(t *testing.T) {
	tests := []struct {
		chars int
		want  int
	}{
		{0, 0},
		{-5, 0},
		{1, 0},
		{charsPerTokenEstimate - 1, 0},
		{charsPerTokenEstimate, 1},
		{charsPerTokenEstimate * 10, 10},
	}
	for _, tt := range tests {
		if got := estimatedOutputTokens(tt.chars); got != tt.want {
			t.Errorf("estimatedOutputTokens(%d) = %d, want %d", tt.chars, got, tt.want)
		}
	}
}

// TestRenderIndicatorGlyphsFixedWidth covers the spec's "a fixed-width run
// of glyphs" — indicatorWidth cells regardless of frame.
func TestRenderIndicatorGlyphsFixedWidth(t *testing.T) {
	for _, frame := range []int{0, 1, 50} {
		got := lipgloss.Width(renderIndicatorGlyphs(theme.Frappe, frame))
		if got != indicatorWidth {
			t.Errorf("frame=%d: renderIndicatorGlyphs width = %d, want %d", frame, got, indicatorWidth)
		}
	}
}

// TestRenderIndicatorGlyphsChangesAcrossFrames covers "continuously
// cycles/scrambles": two different (post-fade-in) frames must render
// differently, not a static single frame reused every tick.
func TestRenderIndicatorGlyphsChangesAcrossFrames(t *testing.T) {
	a := renderIndicatorGlyphs(theme.Frappe, 50)
	b := renderIndicatorGlyphs(theme.Frappe, 51)
	if a == b {
		t.Error("renderIndicatorGlyphs rendered identically for two consecutive frames, want the scramble to change")
	}
}

// TestRenderIndicatorGlyphsGradientReflectsPalette covers the spec's "the
// gradient's colors must come from the active theme.Palette... and changes
// if the theme changes" — Frappe (dark) and Latte (light) have different
// accent hex values, so a fully-revealed frame must render differently
// between them.
func TestRenderIndicatorGlyphsGradientReflectsPalette(t *testing.T) {
	frappe := renderIndicatorGlyphs(theme.Frappe, 100)
	latte := renderIndicatorGlyphs(theme.Latte, 100)
	if frappe == latte {
		t.Error("renderIndicatorGlyphs rendered identically for Frappe and Latte, want the gradient to reflect the active palette")
	}
}

// TestRevealProgressStaggersFadeIn covers "new glyphs fade in... rather
// than popping in all at once": a later cell's reveal starts strictly
// later than an earlier cell's, and every cell eventually reaches full
// opacity.
func TestRevealProgressStaggersFadeIn(t *testing.T) {
	if got := revealProgress(0, 0); got != 0 {
		t.Errorf("revealProgress(0, cell=0) = %v, want 0 (not yet revealed at frame 0)", got)
	}
	if got := revealProgress(0, indicatorWidth-1); got != 0 {
		t.Errorf("revealProgress(0, cell=%d) = %v, want 0", indicatorWidth-1, got)
	}

	// A cell further to the right must not be further along than an
	// earlier cell at the same frame — the reveal sweeps left-to-right.
	midFrame := indicatorWidth * indicatorRevealStagger
	if early, late := revealProgress(midFrame, 0), revealProgress(midFrame, indicatorWidth-1); late > early {
		t.Errorf("revealProgress(%d, last cell) = %v > revealProgress(%d, first cell) = %v, want the last cell no further along", midFrame, late, midFrame, early)
	}

	// Every cell reaches full opacity eventually and stays there.
	for cell := 0; cell < indicatorWidth; cell++ {
		if got := revealProgress(1000, cell); got != 1 {
			t.Errorf("revealProgress(1000, cell=%d) = %v, want 1 (steady state)", cell, got)
		}
	}
}

// TestRenderTurnIndicatorOmitsToolNameBetweenCalls covers the AC that no
// tool name is shown during plain text generation, between calls.
func TestRenderTurnIndicatorOmitsToolNameBetweenCalls(t *testing.T) {
	got := renderTurnIndicator(theme.Frappe, 0, 3*time.Second, "", 42)
	if strings.Count(got, "·") != 1 {
		t.Errorf("renderTurnIndicator with no tool = %q, want exactly one \"·\" separator (elapsed · tokens, no tool segment)", got)
	}
	if !strings.Contains(got, "3s") {
		t.Errorf("renderTurnIndicator = %q, want it to contain the elapsed time %q", got, "3s")
	}
	if !strings.Contains(got, "42") {
		t.Errorf("renderTurnIndicator = %q, want it to contain the token estimate", got)
	}
}

// TestRenderTurnIndicatorIncludesToolNameDuringCall covers the AC that the
// in-flight tool's name is shown while a tool call is executing.
func TestRenderTurnIndicatorIncludesToolNameDuringCall(t *testing.T) {
	got := renderTurnIndicator(theme.Frappe, 0, 3*time.Second, "read_file", 42)
	if !strings.Contains(got, "read_file") {
		t.Errorf("renderTurnIndicator = %q, want it to contain the active tool's name", got)
	}
	if strings.Count(got, "·") != 2 {
		t.Errorf("renderTurnIndicator with a tool = %q, want exactly two \"·\" separators (elapsed · tool · tokens)", got)
	}
}

// TestSyncViewportDimsReservesIndicatorHeightWhileBusy mirrors
// TestSyncViewportDimsReservesPopupDialogHeightWhenActive (popup_test.go):
// the indicator's row is carved out of the viewport's height budget only
// while a turn is actually in progress.
func TestSyncViewportDimsReservesIndicatorHeightWhileBusy(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	before := m.viewport.Height()

	m.busy = true
	m.syncViewportDims()
	if got, want := m.viewport.Height(), before-indicatorHeight; got != want {
		t.Errorf("busy: viewport.Height() = %d, want %d (indicatorHeight reserved)", got, want)
	}

	m.busy = false
	m.syncViewportDims()
	if got := m.viewport.Height(); got != before {
		t.Errorf("idle: viewport.Height() = %d, want %d (no indicator reserved)", got, before)
	}
}

// TestViewCursorRowIncludesIndicatorHeightWhenBusy mirrors
// TestViewCursorRowIncludesPopupDialogHeightWhenActive: since the
// indicator now sits between the transcript/popup and the input, the
// input's on-screen row (and so the cursor's) shifts down by
// indicatorHeight while a turn is in progress.
func TestViewCursorRowIncludesIndicatorHeightWhenBusy(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	m.input.Focus()
	m.input.SetVirtualCursor(false)
	m.busy = true
	m.turnStart = time.Now()
	m.syncViewportDims()

	cur := m.input.Cursor()
	if cur == nil {
		t.Fatal("input.Cursor() = nil, want a cursor (input is focused)")
	}

	v := m.View()
	want := cur.Y + m.viewport.Height() + 1 + indicatorHeight + inputBorderTopHeight
	if v.Cursor == nil || v.Cursor.Y != want {
		t.Errorf("View().Cursor.Y = %v, want %d (indicatorHeight added while busy)", v.Cursor, want)
	}
}

// TestViewShowsIndicatorOnlyWhileBusy covers the top-level AC: the
// indicator's content (its token-count readout is a marker no other part
// of the view renders) is present in View() exactly when busy is true.
func TestViewShowsIndicatorOnlyWhileBusy(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)

	if strings.Contains(m.View().Content, "tokens") {
		t.Error("idle View() contains the indicator's token readout, want it hidden while not busy")
	}

	m.busy = true
	m.turnStart = time.Now()
	if !strings.Contains(m.View().Content, "tokens") {
		t.Error("busy View() doesn't contain the indicator's token readout, want it visible while busy")
	}
}

// TestSubmitShowsIndicatorThenHidesItOnCompletion is an end-to-end pass
// through submit()/drain(): busy (and so the indicator) comes up the
// instant a turn starts and goes away the instant it ends, matching busy's
// role as the single show/hide gate.
func TestSubmitShowsIndicatorThenHidesItOnCompletion(t *testing.T) {
	fp := &multiCallProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "hi"}, provider.DoneEvent{FinishReason: "stop"}},
	}}
	m := New(agent.Loop{Provider: fp}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	m.input.SetValue("hi")

	next, cmd := m.submit()
	mm := next.(Model)
	if !mm.busy {
		t.Fatal("busy = false right after submit, want true")
	}
	if !strings.Contains(mm.View().Content, "tokens") {
		t.Error("View() right after submit doesn't show the indicator, want it visible while busy")
	}

	final := drain(t, mm, cmd)
	if final.busy {
		t.Fatal("busy = true after the turn finished, want false")
	}
	if strings.Contains(final.View().Content, "tokens") {
		t.Error("View() after the turn finished still shows the indicator, want it hidden")
	}
}

// TestHandleEventTracksActiveToolAcrossACall covers the spec's "track
// currently executing tool name as new state set on ToolCallEvent and
// cleared on ToolResultEvent" — no tool name is tracked between calls.
func TestHandleEventTracksActiveToolAcrossACall(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	if m.activeTool != "" {
		t.Fatalf("activeTool = %q before any event, want empty", m.activeTool)
	}

	m.handleEvent(provider.ToolCallEvent{ID: "call_1", Name: "read_file", ArgsJSON: `{}`})
	if m.activeTool != "read_file" {
		t.Errorf("activeTool after ToolCallEvent = %q, want %q", m.activeTool, "read_file")
	}

	m.handleEvent(provider.ToolResultEvent{ID: "call_1", Name: "read_file", Content: "ok"})
	if m.activeTool != "" {
		t.Errorf("activeTool after ToolResultEvent = %q, want empty (cleared on result)", m.activeTool)
	}
}

// TestHandleEventAccumulatesTurnOutputCharsAndResetsOnDone covers the
// spec's "estimate output tokens from the text arriving via TextDeltaEvent
// ... once the turn's real DoneEvent.Usage arrives, the estimate should be
// replaced by the authoritative figure, not left as a stale guess."
func TestHandleEventAccumulatesTurnOutputCharsAndResetsOnDone(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)

	m.handleEvent(provider.TextDeltaEvent{Text: "hello "})
	m.handleEvent(provider.TextDeltaEvent{Text: "world"})
	if want := len([]rune("hello world")); m.turnOutputChars != want {
		t.Fatalf("turnOutputChars = %d, want %d after two TextDeltaEvents", m.turnOutputChars, want)
	}

	m.handleEvent(provider.DoneEvent{Usage: provider.Usage{OutputTokens: 7}})
	if m.turnOutputChars != 0 {
		t.Errorf("turnOutputChars after DoneEvent = %d, want 0 (superseded by the authoritative Usage)", m.turnOutputChars)
	}
	if m.sess.TotalOutputTokens != 7 {
		t.Errorf("sess.TotalOutputTokens = %d, want 7", m.sess.TotalOutputTokens)
	}
}

// TestFinishTurnClearsIndicatorState covers stopIndicator's role in
// finishTurn: a completed (or errored/interrupted) turn must not leave a
// stale tool name or token count behind for the next busy window to
// momentarily flash.
func TestFinishTurnClearsIndicatorState(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.busy = true
	m.activeTool = "read_file"
	m.turnOutputChars = 123

	m.finishTurn(nil, errors.New("boom"))

	if m.busy {
		t.Error("busy = true after finishTurn, want false")
	}
	if m.activeTool != "" {
		t.Errorf("activeTool = %q after finishTurn, want empty", m.activeTool)
	}
	if m.turnOutputChars != 0 {
		t.Errorf("turnOutputChars = %d after finishTurn, want 0", m.turnOutputChars)
	}
}

// TestIndicatorTickMsgStopsReschedulingOnceIdle covers "the indicator must
// not... delay Escape-driven interruption": once busy goes false (e.g. via
// Escape's cancelTurn -> finishTurn path), the next indicatorTickMsg to
// arrive must not reschedule another one, so no tick loop lingers past the
// turn's end.
func TestIndicatorTickMsgStopsReschedulingOnceIdle(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.busy = false

	next, cmd := m.Update(indicatorTickMsg{})
	mm := next.(Model)
	if cmd != nil {
		t.Error("Update(indicatorTickMsg{}) while idle returned a non-nil cmd, want nil (stop rescheduling)")
	}
	if mm.animFrame != 0 {
		t.Errorf("animFrame = %d after an idle tick, want 0 (unchanged)", mm.animFrame)
	}
}

// TestIndicatorTickMsgAdvancesFrameAndReschedulesWhileBusy covers the
// animation's live-ticking loop: each tick bumps animFrame and schedules
// the next one, for as long as busy stays true.
func TestIndicatorTickMsgAdvancesFrameAndReschedulesWhileBusy(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.busy = true

	next, cmd := m.Update(indicatorTickMsg{})
	mm := next.(Model)
	if mm.animFrame != 1 {
		t.Errorf("animFrame = %d after one busy tick, want 1", mm.animFrame)
	}
	if cmd == nil {
		t.Error("Update(indicatorTickMsg{}) while busy returned a nil cmd, want the next tick rescheduled")
	}
}

// TestCompactShowsIndicatorWhileBusy covers /compact's own busy window
// (compact.go), which reuses the same indicator machinery as a regular
// turn rather than a parallel mechanism.
func TestCompactShowsIndicatorWhileBusy(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	m.sess.Messages = []provider.Message{{Role: "user", Content: "hi"}}

	next, cmd := m.compact()
	mm := next.(Model)
	if !mm.busy {
		t.Fatal("busy = false right after compact(), want true")
	}
	if !strings.Contains(mm.View().Content, "tokens") {
		t.Error("View() right after compact() doesn't show the indicator, want it visible while busy")
	}

	final := drain(t, mm, cmd)
	if final.busy {
		t.Fatal("busy = true after compaction finished, want false")
	}
}

// TestSubmitDispatchesToolCallShowsActiveToolNameLive confirms activeTool
// set by a streamed ToolCallEvent actually reaches View() while the call is
// in flight, tying TestHandleEventTracksActiveToolAcrossACall's state
// transition to what the user would actually see on screen.
func TestSubmitDispatchesToolCallShowsActiveToolNameLive(t *testing.T) {
	ft := &fakeTool{name: "read", result: tool.Result{Content: "file content"}}
	fp := &multiCallProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "read", ArgsJSON: `{"path":"foo"}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	m := New(agent.Loop{Provider: fp, Tools: tool.NewRegistry(ft)}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	m.input.SetValue("read foo")

	next, cmd := m.submit()
	mm := next.(Model)

	// Mirror the ToolCallEvent the fake provider is about to stream, so
	// View() can be inspected mid-flight rather than only after drain()
	// runs the whole turn to completion.
	mm.handleEvent(provider.ToolCallEvent{ID: "call_1", Name: "read", ArgsJSON: `{"path":"foo"}`})
	if !strings.Contains(mm.View().Content, "read") {
		t.Errorf("View() while the tool call is in flight doesn't mention %q", "read")
	}

	drain(t, mm, cmd)
}
