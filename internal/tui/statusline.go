package tui

// This file holds the customizable statusLine (issue #60): a status block
// rendered below the input line, as the screen's bottom footer (issue
// #123), refreshed on session start, after each response, after each tool
// call, and a periodic timer (1s by default, issue #146) — every trigger
// debounced at 300ms via statusGen/statusRefreshMsg/statusRenderedMsg's
// generation check, so a burst of triggers within the debounce window only
// actually runs the (potentially external-process) render once.
// Model's own fields, construction, and the Update/View integration
// points this plugs into stay in tui.go, matching mention.go/history.go's
// own split.

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/statusline"
)

// defaultStatusDebounce is the spec'd 300ms debounce every statusLine
// refresh trigger goes through.
const defaultStatusDebounce = 300 * time.Millisecond

// statusRefreshMsg fires statusDebounce after a statusLine refresh was
// requested. gen is checked against Model.statusGen at both this message
// and statusRenderedMsg: a later request bumps statusGen, so an
// in-flight, now-superseded refresh is silently dropped rather than
// clobbering a fresher one — the debounce mechanism itself.
type statusRefreshMsg struct{ gen int }

// statusRenderedMsg carries one statusLine refresh's result back from the
// background goroutine that built it (which may have run an external
// process). warn, when non-empty, is the command's failure reason,
// surfaced as a system scrollback line the same way MCP load errors are.
type statusRenderedMsg struct {
	gen   int
	lines []string
	warn  string
}

// statusTimerMsg fires every statusline.RefreshInterval — 1s by default,
// even when config.StatusLineConfig.RefreshInterval is unset (issue #146),
// or the configured value otherwise — requesting another refresh (itself
// still subject to the same debounce) and rescheduling itself.
type statusTimerMsg struct{}

// statusTick schedules gen's debounce tick, statusDebounce from now.
func (m Model) statusTick(gen int) tea.Cmd {
	return tea.Tick(m.statusDebounce, func(time.Time) tea.Msg {
		return statusRefreshMsg{gen: gen}
	})
}

// scheduleStatusTimer schedules the next statusTimerMsg: at
// statusline.DefaultRefreshInterval when config.StatusLineConfig.
// RefreshInterval is unset, at the configured interval when it isn't, or
// nil (no timer) when explicitly configured to 0 or less.
func (m Model) scheduleStatusTimer() tea.Cmd {
	interval := statusline.RefreshInterval(m.statusCfg.RefreshInterval)
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg { return statusTimerMsg{} })
}

// requestStatusRefresh bumps statusGen (superseding any refresh already
// in flight for the previous generation) and schedules the new
// generation's debounce tick — the single mechanism behind every
// statusLine refresh trigger (session start, after each response, after
// each tool call, the optional timer, and /clear).
func (m *Model) requestStatusRefresh() tea.Cmd {
	m.statusGen++
	return m.statusTick(m.statusGen)
}

// statusRefreshCmd builds the Cmd that actually renders one statusLine
// refresh: it captures the fields it needs by value/pointer at call time
// (so it reflects state as of when the refresh runs, not when it was
// requested) and runs statusline.BuildInput/Render in the background,
// since a configured command means this may spawn a subprocess.
func (m Model) statusRefreshCmd(gen int) tea.Cmd {
	sess := m.sess
	lookup := m.loop.ContextLookup
	tracker := m.statusTracker
	cwd := m.cwd
	model := m.reqModel
	command := m.statusCfg.Command
	cols, lines := m.width, m.height
	pal := m.pal

	return func() tea.Msg {
		ctx := context.Background()
		in := statusline.BuildInput(ctx, sess, lookup, tracker, cwd, model)
		var warn string
		rows := statusline.Render(ctx, command, in, cols, lines, pal, func(msg string) { warn = msg })
		return statusRenderedMsg{gen: gen, lines: rows, warn: warn}
	}
}

// renderStatusBlock renders the statusLine's current rows, one per line,
// each already palette-styled/truncated by statusline.Render — nothing to
// add here beyond joining them and a trailing separator, matching
// renderTranscript's own trailing-blank-row convention. "" (no rows yet,
// e.g. before the very first refresh completes) renders as nothing.
func (m Model) renderStatusBlock() string {
	if len(m.statusLines) == 0 {
		return ""
	}
	return strings.Join(m.statusLines, "\n") + "\n"
}
