package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/statusline"
)

// drainStatus pumps cmd (and whatever it batches/returns) until a
// statusRenderedMsg is processed, returning the resulting Model. Unlike
// the shared drain helper (which stops at turnDoneMsg, deliberately
// leaving a batched statusLine refresh's own debounce tick unpumped so
// most tests don't pay its real sleep), this drains exactly that refresh
// chain — for tests that want to assert on the rendered result itself.
func drainStatus(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	pending := []tea.Cmd{cmd}
	for len(pending) > 0 {
		cmd, pending = pending[0], pending[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			pending = append(pending, batch...)
			continue
		}
		next, newCmd := m.Update(msg)
		m = next.(Model)
		if _, ok := msg.(statusRenderedMsg); ok {
			return m
		}
		if newCmd != nil {
			pending = append(pending, newCmd)
		}
	}
	t.Fatal("drainStatus: ran out of pending commands before a statusRenderedMsg arrived")
	return m
}

func TestNewThreadsStatusLineConfig(t *testing.T) {
	cfg := config.Config{StatusLine: config.StatusLineConfig{Command: "my-status-command", RefreshInterval: 2000}}
	m := New(agent.Loop{}, cfg, nil)
	if m.statusCfg != cfg.StatusLine {
		t.Errorf("statusCfg = %+v, want %+v", m.statusCfg, cfg.StatusLine)
	}
}

// TestHandleEventTriggersRefreshOnToolResultAndDoneOnly covers the spec'd
// refresh triggers: "after each tool call" and "after each response" — a
// text delta or a still-pending tool-call request must not trigger one.
func TestHandleEventTriggersRefreshOnToolResultAndDoneOnly(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)

	cases := []struct {
		name        string
		ev          provider.Event
		wantRefresh bool
	}{
		{"text delta", provider.TextDeltaEvent{Text: "x"}, false},
		{"tool call requested", provider.ToolCallEvent{ID: "1", Name: "read"}, false},
		{"tool result", provider.ToolResultEvent{ID: "1", Name: "read", Content: "ok"}, true},
		{"done", provider.DoneEvent{FinishReason: "stop"}, true},
	}
	for _, tc := range cases {
		if got := m.handleEvent(tc.ev); got != tc.wantRefresh {
			t.Errorf("%s: handleEvent() = %v, want %v", tc.name, got, tc.wantRefresh)
		}
	}

	in := statusline.BuildInput(context.Background(), m.sess, nil, m.statusTracker, "", "")
	if in.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1 (one ToolResultEvent handled)", in.ToolCalls)
	}
}

// TestSubmitSlashClearResetsStatusTracker covers /clear's fresh-session
// semantics extending to the statusLine tracker: tool-call count goes back
// to 0, matching session.Session's own cost/context reset.
func TestSubmitSlashClearResetsStatusTracker(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.statusDebounce = 0
	m.statusTracker.RecordToolCall()
	m.statusTracker.RecordToolCall()
	m.input.SetValue("/clear")

	next, _ := m.submit()
	mm := next.(Model)

	in := statusline.BuildInput(context.Background(), mm.sess, nil, mm.statusTracker, "", "")
	if in.ToolCalls != 0 {
		t.Errorf("ToolCalls after /clear = %d, want 0", in.ToolCalls)
	}
}

// TestRequestStatusRefreshDropsSupersededGeneration covers the debounce
// mechanism itself: a burst of refresh requests must only ever actually
// render the latest one — an earlier request's tick, once it fires, must
// see it's been superseded and do nothing.
func TestRequestStatusRefreshDropsSupersededGeneration(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.statusDebounce = 0

	staleCmd := m.requestStatusRefresh()
	latestCmd := m.requestStatusRefresh()

	next, staleFollowUp := m.Update(staleCmd())
	m = next.(Model)
	if staleFollowUp != nil {
		t.Error("a superseded statusRefreshMsg triggered a render, want it dropped")
	}

	next, latestFollowUp := m.Update(latestCmd())
	m = next.(Model)
	if latestFollowUp == nil {
		t.Fatal("the latest statusRefreshMsg was dropped, want it to trigger a render")
	}
}

// TestStaleStatusRenderedMsgIsDropped covers the other half of the
// debounce/staleness mechanism: a render that was already in flight when a
// newer refresh got requested must not clobber statusLines once it
// eventually completes.
func TestStaleStatusRenderedMsgIsDropped(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.statusLines = []string{"old"}
	m.statusGen = 5

	next, _ := m.Update(statusRenderedMsg{gen: 4, lines: []string{"new-but-stale"}})
	mm := next.(Model)

	if len(mm.statusLines) != 1 || mm.statusLines[0] != "old" {
		t.Errorf("statusLines = %v, want unchanged (a stale render must be dropped)", mm.statusLines)
	}
}

func TestSyncViewportDimsReservesRowsForStatusLines(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m = next.(Model)
	before := m.viewport.Height()

	m.statusLines = []string{"row one", "row two"}
	m.syncViewportDims()

	if got, want := m.viewport.Height(), before-2; got != want {
		t.Errorf("viewport.Height() = %d, want %d (2 rows reserved for the status block)", got, want)
	}
}

func TestRenderStatusBlockEmptyWhenNoRows(t *testing.T) {
	var m Model
	if got := m.renderStatusBlock(); got != "" {
		t.Errorf("renderStatusBlock() = %q, want empty with no rows", got)
	}
}

func TestRenderStatusBlockJoinsRowsWithTrailingNewline(t *testing.T) {
	m := Model{statusLines: []string{"a", "b"}}
	if got, want := m.renderStatusBlock(), "a\nb\n"; got != want {
		t.Errorf("renderStatusBlock() = %q, want %q", got, want)
	}
}

// TestViewPlacesStatusBlockAboveInput covers the spec's layout: the status
// block sits between the transcript and the input line, not top-of-screen.
func TestViewPlacesStatusBlockAboveInput(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	m.statusLines = []string{"STATUSMARKER"}

	content := m.View().Content
	statusIdx := strings.Index(content, "STATUSMARKER")
	inputIdx := strings.Index(content, m.input.View())
	if statusIdx == -1 {
		t.Fatal("View() content doesn't include the status block")
	}
	if inputIdx == -1 || statusIdx >= inputIdx {
		t.Errorf("status block (at %d) is not positioned above the input line (at %d)", statusIdx, inputIdx)
	}
}

// TestStatusLineRefreshEndToEndUsesConfiguredCommandAndTerminalSize drives
// the full refresh pipeline — request, debounce tick, background render —
// against a real configured command, asserting on both the stdin/env
// wiring (COLUMNS/LINES) and the resulting statusLines.
func TestStatusLineRefreshEndToEndUsesConfiguredCommandAndTerminalSize(t *testing.T) {
	cfg := config.Config{StatusLine: config.StatusLineConfig{Command: `echo "$COLUMNS,$LINES"`}}
	m := New(agent.Loop{}, cfg, nil)
	m.statusDebounce = 0
	next, _ := m.Update(tea.WindowSizeMsg{Width: 42, Height: 24})
	m = next.(Model)

	final := drainStatus(t, m, m.requestStatusRefresh())

	if len(final.statusLines) != 1 || final.statusLines[0] != "42,24" {
		t.Errorf("statusLines = %v, want [\"42,24\"]", final.statusLines)
	}
}

// TestStatusLineRefreshEndToEndFailingCommandWarnsInTranscript covers a
// configured command's failure path: no rows, and the failure reason
// surfaces as a system scrollback line (matching the MCP-load-error
// convention).
func TestStatusLineRefreshEndToEndFailingCommandWarnsInTranscript(t *testing.T) {
	cfg := config.Config{StatusLine: config.StatusLineConfig{Command: `echo "boom" >&2; exit 1`}}
	m := New(agent.Loop{}, cfg, nil)
	m.statusDebounce = 0

	final := drainStatus(t, m, m.requestStatusRefresh())

	if final.statusLines != nil {
		t.Errorf("statusLines = %v, want nil after a failing command", final.statusLines)
	}
	found := false
	for _, l := range final.lines {
		if l.role == "system" && strings.Contains(l.text, "boom") {
			found = true
		}
	}
	if !found {
		t.Errorf("lines = %+v, want a system line mentioning the command's stderr", final.lines)
	}
}

// TestStatusLineRefreshEndToEndDefaultRendererUsesCwdAndModel covers the
// built-in default renderer (no configured command), threaded with
// WithCwd and the configured provider.model.
func TestStatusLineRefreshEndToEndDefaultRendererUsesCwdAndModel(t *testing.T) {
	cfg := config.Config{Provider: config.ProviderConfig{Model: "openrouter/auto"}}
	m := New(agent.Loop{}, cfg, nil).WithCwd("/proj")
	m.statusDebounce = 0

	final := drainStatus(t, m, m.requestStatusRefresh())

	if len(final.statusLines) != 2 {
		t.Fatalf("statusLines = %v, want 2 rows (identity + metrics)", final.statusLines)
	}
	if !strings.Contains(final.statusLines[0], "openrouter/auto") || !strings.Contains(final.statusLines[0], "/proj") {
		t.Errorf("identity row = %q, want it to mention the model and cwd", final.statusLines[0])
	}
}
