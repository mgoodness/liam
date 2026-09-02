package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
)

// sized returns m resized to a small window (forcing the transcript to
// scroll) and populated with n system lines, refreshed into the viewport.
func sized(t *testing.T, m Model, n int) Model {
	t.Helper()
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick for the handful of callers that go on to submit()+drain()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = next.(Model)
	for i := range n {
		m.lines = append(m.lines, line{role: "system", text: fmt.Sprintf("line %d", i)})
	}
	m.refreshViewport()
	if !m.followBottom || !m.viewport.AtBottom() {
		t.Fatal("sized: expected auto-follow to leave the viewport at the bottom")
	}
	return m
}

func TestPageUpPagesAwayFromBottomAndPinsPosition(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	mm := next.(Model)
	if mm.viewport.AtBottom() {
		t.Error("pgup did not scroll the viewport away from the bottom")
	}
	if mm.followBottom {
		t.Error("pgup left followBottom = true, want false (manual scroll pins the position)")
	}
}

// TestPageDownAtBottomIsNoOpAndDoesNotBreakFollow covers pinIfScrolled's
// no-op guard: pressing PageDown while already following at the bottom
// must not spuriously pin the position, since the scroll didn't move
// anything.
func TestPageDownAtBottomIsNoOpAndDoesNotBreakFollow(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	mm := next.(Model)
	if !mm.followBottom {
		t.Error("a no-op pgdown (already at the bottom) broke auto-follow, want it left alone")
	}
	if !mm.viewport.AtBottom() {
		t.Error("a no-op pgdown moved the viewport, want it to stay at the bottom")
	}
}

// TestPageDownReachingBottomResumesAutoFollow is a regression test: paging
// back down to the exact bottom row (without pressing End) must resume
// auto-follow on its own, the same as End does — a scroll that overshoots
// past the bottom and only settles there is indistinguishable, from the
// reader's perspective, from explicitly asking to jump to the end.
func TestPageDownReachingBottomResumesAutoFollow(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	mm := next.(Model)
	if mm.followBottom {
		t.Fatal("pgup did not pin the position; test setup invalid")
	}

	for range 10 {
		next, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		mm = next.(Model)
		if mm.viewport.AtBottom() {
			break
		}
	}
	if !mm.viewport.AtBottom() {
		t.Fatal("pgdown never reached the bottom")
	}
	if !mm.followBottom {
		t.Error("reaching the bottom via pgdown alone did not resume auto-follow")
	}
}

func TestCtrlUCtrlDHalfPageScroll(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)
	startOffset := m.viewport.YOffset()

	next, _ := m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	mm := next.(Model)
	if mm.viewport.YOffset() >= startOffset {
		t.Errorf("ctrl+u did not scroll up: offset %d, want < %d", mm.viewport.YOffset(), startOffset)
	}
	if mm.followBottom {
		t.Error("ctrl+u left followBottom = true, want false")
	}

	next, _ = mm.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	after := next.(Model)
	if after.viewport.YOffset() <= mm.viewport.YOffset() {
		t.Errorf("ctrl+d did not scroll down: offset %d, want > %d", after.viewport.YOffset(), mm.viewport.YOffset())
	}
}

// TestPlainJKHLUpDownAreNotBoundToScrolling covers the AC that liam
// intercepts only PageUp/PageDown/Ctrl+U/Ctrl+D directly, never routing
// through the viewport's own default KeyMap (which binds j/k/h/l/Up/Down)
// — those keys must still reach the textarea for typing, and Up/Down must
// still drive history recall.
func TestPlainJKHLUpDownAreNotBoundToScrolling(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)
	offsetBefore := m.viewport.YOffset()

	for _, r := range []rune{'j', 'k', 'h', 'l'} {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}

	if m.viewport.YOffset() != offsetBefore {
		t.Errorf("j/k/h/l changed the viewport offset (%d -> %d), want no scrolling", offsetBefore, m.viewport.YOffset())
	}
	if m.input.Value() != "jkhl" {
		t.Errorf("input.Value() = %q, want %q (j/k/h/l must still type)", m.input.Value(), "jkhl")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	mm := next.(Model)
	if mm.viewport.YOffset() != offsetBefore {
		t.Error("Up changed the viewport offset, want it to drive history recall/cursor movement instead")
	}
}

func TestMouseWheelScrollsViewportAndPinsPosition(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	mm := next.(Model)
	if mm.viewport.AtBottom() {
		t.Error("mouse wheel up did not scroll the viewport away from the bottom")
	}
	if mm.followBottom {
		t.Error("mouse wheel up left followBottom = true, want false")
	}
	upOffset := mm.viewport.YOffset()

	next, _ = mm.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	after := next.(Model)
	if after.viewport.YOffset() <= upOffset {
		t.Errorf("mouse wheel down did not scroll back down: offset %d, want > %d", after.viewport.YOffset(), upOffset)
	}
}

func TestEndKeyResumesAutoFollow(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	mm := next.(Model)
	if mm.followBottom {
		t.Fatal("pgup did not pin the position; test setup invalid")
	}

	next, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	after := next.(Model)
	if !after.followBottom {
		t.Error("End did not resume auto-follow")
	}
	if !after.viewport.AtBottom() {
		t.Error("End did not jump the viewport back to the bottom")
	}
}

// TestStreamedContentAutoScrollsWhileFollowing covers the AC that the
// viewport auto-scrolls to the bottom as new content streams in, as long
// as no manual scroll has pinned the position.
func TestStreamedContentAutoScrollsWhileFollowing(t *testing.T) {
	fp := &multiCallProvider{turns: [][]provider.Event{{
		provider.TextDeltaEvent{Text: "one "},
		provider.TextDeltaEvent{Text: "two "},
		provider.TextDeltaEvent{Text: "three"},
		provider.DoneEvent{FinishReason: "stop"},
	}}}
	m := New(agent.Loop{Provider: fp}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = next.(Model)
	for i := range 30 {
		m.lines = append(m.lines, line{role: "system", text: fmt.Sprintf("line %d", i)})
	}
	m.refreshViewport()
	m.input.SetValue("go")

	next, cmd := m.submit()
	mm := next.(Model)
	if !mm.viewport.AtBottom() {
		t.Fatal("submitting did not keep the viewport pinned to the bottom")
	}

	final := drain(t, mm, cmd)
	if !final.viewport.AtBottom() {
		t.Error("viewport did not auto-scroll to the bottom as the response streamed in")
	}
}

// TestManualScrollPinsPositionDuringStreaming covers the other half of the
// same AC: once a manual scroll has pinned the position, streamed content
// must not yank the view back to the bottom.
func TestManualScrollPinsPositionDuringStreaming(t *testing.T) {
	fp := &multiCallProvider{turns: [][]provider.Event{{
		provider.TextDeltaEvent{Text: "one"},
		provider.DoneEvent{FinishReason: "stop"},
	}}}
	m := sized(t, New(agent.Loop{Provider: fp}, config.Config{}, nil), 30)
	m.input.SetValue("go")

	next, cmd := m.submit()
	mm := next.(Model)

	scrolled, _ := mm.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	pinned := scrolled.(Model)
	pinnedOffset := pinned.viewport.YOffset()
	if pinned.followBottom {
		t.Fatal("pgup did not pin the position; test setup invalid")
	}

	final := drain(t, pinned, cmd)
	if final.followBottom {
		t.Error("streamed content resumed auto-follow despite a manual scroll pinning the position")
	}
	if final.viewport.YOffset() != pinnedOffset {
		t.Errorf("streamed content moved the pinned offset: %d -> %d", pinnedOffset, final.viewport.YOffset())
	}
}

// TestRefreshViewportResyncsDimsWhenInputGrows is a regression test: the
// input's height can grow (e.g. the user starts a multi-line draft) between
// resize events, and refreshViewport must resync the viewport's height
// itself before pinning to the bottom — otherwise GotoBottom lands against
// a stale, too-tall height and the transcript stops tracking the true last
// line until the next resize or an explicit End.
func TestRefreshViewportResyncsDimsWhenInputGrows(t *testing.T) {
	m := sized(t, New(agent.Loop{}, config.Config{}, nil), 30)

	// Insert newlines directly (bypassing handleKey's own resync-on-scroll
	// paths) to grow the textarea without ever going through a
	// WindowSizeMsg — exactly the gap the fix closes.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	mm := next.(Model)
	next, _ = mm.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	mm = next.(Model)
	if mm.input.Height() <= 1 {
		t.Fatalf("input.Height() = %d, want > 1 after inserting newlines", mm.input.Height())
	}

	next, _ = mm.Update(systemLineMsg{text: "note"})
	after := next.(Model)

	wantHeight := max(0, after.height-after.input.Height()-inputBorderHeight-1)
	if after.viewport.Height() != wantHeight {
		t.Errorf("viewport.Height() = %d, want %d (resynced from the grown input)", after.viewport.Height(), wantHeight)
	}
	if !after.viewport.AtBottom() {
		t.Error("refreshViewport did not land on the true bottom after the input grew")
	}
}
