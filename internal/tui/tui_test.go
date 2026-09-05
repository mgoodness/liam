package tui

import (
	"context"
	"errors"
	"image/color"
	"iter"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/skill"
	"github.com/mgoodness/liam/internal/tool"
)

// multiCallProvider scripts one []provider.Event per call, advancing by
// call order (needed for the tool-call test, which spans two turns).
type multiCallProvider struct {
	turns [][]provider.Event
	calls int
}

func (f *multiCallProvider) Name() string { return "fake-multi" }

func (f *multiCallProvider) Stream(_ context.Context, _ provider.Request) iter.Seq2[provider.Event, error] {
	idx := f.calls
	f.calls++
	return func(yield func(provider.Event, error) bool) {
		for _, ev := range f.turns[idx] {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// drain repeatedly invokes pending commands and feeds each resulting Msg
// back through Update, exactly as a real tea.Program would, until a
// turnDoneMsg or compactDoneMsg is processed — at which point it returns
// immediately, leaving any other still-pending commands (e.g. a batched
// statusLine refresh's debounce tick) uninvoked, same as every other test
// in this package that never drives Update far enough to reach one. A tea.
// BatchMsg (concurrent commands returned via tea.Batch, e.g. streamMsg's
// event-wait batched with a statusLine refresh trigger) is unpacked into
// the same worklist rather than fed to Update directly, mirroring how the
// real runtime executes each of a batch's commands independently.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("drain: nil cmd, nothing to pump")
	}
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
		switch msg.(type) {
		case turnDoneMsg, compactDoneMsg:
			return m
		}
		if newCmd != nil {
			pending = append(pending, newCmd)
		}
	}
	return m
}

func TestSubmitStreamsResponseAndAppendsAssistantLine(t *testing.T) {
	fp := &multiCallProvider{turns: [][]provider.Event{
		{
			provider.TextDeltaEvent{Text: "hel"},
			provider.TextDeltaEvent{Text: "lo"},
			provider.DoneEvent{FinishReason: "stop", Usage: provider.Usage{CostUSD: 0.01}},
		},
	}}
	m := New(agent.Loop{Provider: fp}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	m.input.SetValue("hi there")

	next, cmd := m.submit()
	mm := next.(Model)
	if !mm.busy {
		t.Fatal("busy = false right after submit, want true")
	}
	if len(mm.lines) != 1 || mm.lines[0].role != "user" || mm.lines[0].text != "hi there" {
		t.Fatalf("lines after submit = %+v, want a single user line", mm.lines)
	}

	final := drain(t, mm, cmd)

	if final.busy {
		t.Error("busy = true after turn finished, want false")
	}
	if len(final.lines) != 3 {
		t.Fatalf("lines after turn = %+v, want 3 (user + assistant + completion)", final.lines)
	}
	if final.lines[1].role != "assistant" || final.lines[1].text != "hello" {
		t.Errorf("lines[1] = %+v, want assistant %q", final.lines[1], "hello")
	}
	if final.lines[2].role != "complete" {
		t.Errorf("lines[2].role = %q, want %q", final.lines[2].role, "complete")
	}
	if final.sess.CostUSD != 0.01 {
		t.Errorf("sess.CostUSD = %v, want 0.01", final.sess.CostUSD)
	}
	if len(final.sess.Messages) != 2 {
		t.Fatalf("sess.Messages = %+v, want 2 (user + assistant)", final.sess.Messages)
	}
}

// capturingProvider records the last Request it received and immediately
// completes with a DoneEvent — enough to inspect what submit() built
// without needing to script any streamed content.
type capturingProvider struct {
	lastReq provider.Request
}

func (f *capturingProvider) Name() string { return "fake-capturing" }

func (f *capturingProvider) Stream(_ context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	f.lastReq = req
	return func(yield func(provider.Event, error) bool) {
		yield(provider.DoneEvent{FinishReason: "stop"}, nil)
	}
}

// TestSubmitThreadsSystemPromptFromWithSystemPrompt covers issue #56's
// wiring into the TUI: whatever WithSystemPrompt attached at New(...) time
// (the discovered AGENTS.md/LIAM.md project instructions) must ride along
// on every submitted turn's Request.SystemPrompt.
func TestSubmitThreadsSystemPromptFromWithSystemPrompt(t *testing.T) {
	fp := &capturingProvider{}
	m := New(agent.Loop{Provider: fp}, config.Config{}, nil).WithSystemPrompt("project instructions")
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	m.input.SetValue("hi")

	next, cmd := m.submit()
	mm := next.(Model)
	drain(t, mm, cmd)

	if fp.lastReq.SystemPrompt != "project instructions" {
		t.Errorf("req.SystemPrompt = %q, want %q", fp.lastReq.SystemPrompt, "project instructions")
	}
}

func TestSubmitDispatchesToolCallAndRendersResultLine(t *testing.T) {
	ft := &fakeTool{name: "read", result: tool.Result{Content: "file content"}}
	fp := &multiCallProvider{turns: [][]provider.Event{
		{
			provider.TextDeltaEvent{Text: "looking"},
			provider.ToolCallEvent{ID: "call_1", Name: "read", ArgsJSON: `{"path":"foo"}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{
			provider.TextDeltaEvent{Text: "done"},
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	m := New(agent.Loop{Provider: fp, Tools: tool.NewRegistry(ft)}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	m.input.SetValue("read foo")

	next, cmd := m.submit()
	final := drain(t, next.(Model), cmd)

	// user, "looking" (flushed before the tool call), tool result, "done", completion
	if len(final.lines) != 5 {
		t.Fatalf("lines = %+v, want 5", final.lines)
	}
	if final.lines[1].role != "assistant" || final.lines[1].text != "looking" {
		t.Errorf("lines[1] = %+v, want assistant %q (flushed before the tool call)", final.lines[1], "looking")
	}
	if final.lines[2].role != "tool" {
		t.Errorf("lines[2].role = %q, want %q", final.lines[2].role, "tool")
	}
	wantToolLine := `read(path: "foo") → file content`
	if final.lines[2].text != wantToolLine {
		t.Errorf("lines[2].text = %q, want %q", final.lines[2].text, wantToolLine)
	}
	if final.lines[3].role != "assistant" || final.lines[3].text != "done" {
		t.Errorf("lines[3] = %+v, want assistant %q", final.lines[3], "done")
	}
	if final.lines[4].role != "complete" {
		t.Errorf("lines[4].role = %q, want %q", final.lines[4].role, "complete")
	}
}

func TestSubmitSlashQuitReturnsQuitCmd(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.input.SetValue("/quit")

	_, cmd := m.submit()
	if cmd == nil {
		t.Fatal("submit(\"/quit\") returned a nil cmd, want tea.Quit")
	}
	// config.Config{}'s default theme.mode ("") is "auto", so quitCmd
	// sequences its mode-2031 teardown (issue #203, docs/adr/0018) ahead of
	// tea.Quit rather than returning tea.Quit alone — see
	// TestQuitCmdDisablesColorSchemePushInAutoMode for that sequence's own
	// dedicated ordering coverage.
	steps, ok := sequenceSteps(cmd())
	if !ok {
		t.Fatalf("submit(\"/quit\") cmd produced %#v, want a tea.Sequence (mode-2031 disable, then tea.Quit)", cmd())
	}
	var sawQuit bool
	for _, step := range steps {
		if _, ok := step().(tea.QuitMsg); ok {
			sawQuit = true
		}
	}
	if !sawQuit {
		t.Error("submit(\"/quit\") cmd's sequence didn't include tea.QuitMsg")
	}
}

func TestSubmitSlashClearResetsSessionAndLines(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.statusDebounce = 0 // avoid a real 300ms sleep when the test invokes cmd() below
	m.lines = []line{{role: "user", text: "old"}}
	m.sess.Messages = []provider.Message{{Role: "user", Content: "old"}}
	oldID := m.sess.ID
	m.input.SetValue("/clear")

	next, cmd := m.submit()
	mm := next.(Model)

	// /clear starts no turn, but it does fire a fresh "session start"
	// statusLine refresh (issue #60) — so cmd is the debounce tick for
	// that, not nil.
	if cmd == nil {
		t.Fatal("submit(\"/clear\") returned a nil cmd, want the statusLine session-start refresh")
	}
	if _, ok := cmd().(statusRefreshMsg); !ok {
		t.Errorf("submit(\"/clear\") cmd produced %#v, want a statusRefreshMsg", cmd())
	}
	// The banner (issue #169) reappears as the freshly-cleared transcript's
	// new first line, rather than lines going back to empty.
	if len(mm.lines) == 0 || !strings.Contains(mm.lines[0].text, "Liam") {
		t.Errorf("lines = %+v, want the startup banner as the sole line after /clear", mm.lines)
	}
	if mm.sess.Messages != nil {
		t.Errorf("sess.Messages = %+v, want nil after /clear", mm.sess.Messages)
	}
	if mm.sess.ID == oldID {
		t.Error("/clear did not assign a fresh session ID")
	}
}

// TestNewFiresSessionStartHookAndPointsRunnerAtTheSession covers issue #45's
// sessionStart lifecycle point in the interactive TUI: New must fire it
// immediately, with the shared hook.Runner already pointed at the freshly
// assigned session ID.
func TestNewFiresSessionStartHookAndPointsRunnerAtTheSession(t *testing.T) {
	dir := t.TempDir()
	startedPath := dir + "/started"
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		SessionStart: []config.HookConfig{{Command: "touch " + startedPath}},
	}}

	m := New(agent.Loop{Hooks: hooks}, config.Config{}, nil)

	if !fileExists(startedPath) {
		t.Error("sessionStart hook did not run")
	}
	if hooks.SessionID != m.sess.ID {
		t.Errorf("hooks.SessionID = %q, want the session's ID %q", hooks.SessionID, m.sess.ID)
	}
}

// TestSubmitSlashClearFiresSessionEndThenSessionStart covers /clear's
// lifecycle: the old session's sessionEnd fires, then the new session's
// sessionStart fires with the Runner repointed at the new session ID.
func TestSubmitSlashClearFiresSessionEndThenSessionStart(t *testing.T) {
	dir := t.TempDir()
	endedPath := dir + "/ended"
	startedPath := dir + "/started"
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		SessionStart: []config.HookConfig{{Command: "touch " + startedPath}},
		SessionEnd:   []config.HookConfig{{Command: "touch " + endedPath}},
	}}
	m := New(agent.Loop{Hooks: hooks}, config.Config{}, nil)
	if err := removeFile(startedPath); err != nil {
		t.Fatalf("removing New()'s own sessionStart marker: %v", err)
	}
	oldID := m.sess.ID
	m.input.SetValue("/clear")

	next, _ := m.submit()
	mm := next.(Model)

	if !fileExists(endedPath) {
		t.Error("sessionEnd hook did not run on /clear")
	}
	if !fileExists(startedPath) {
		t.Error("sessionStart hook did not re-run on /clear")
	}
	if hooks.SessionID != mm.sess.ID || hooks.SessionID == oldID {
		t.Errorf("hooks.SessionID = %q, want the new session's ID %q (old was %q)", hooks.SessionID, mm.sess.ID, oldID)
	}
}

// TestSubmitDeniedByUserPromptSubmitHookShowsReasonAndStartsNoTurn covers
// issue #102's userPromptSubmit lifecycle point: a denying hook must show
// the user what they typed plus the denial reason, and start no turn at
// all — the raw text never even reaches the /quit-/clear-/skills-/compact
// dispatch table below it in submit().
func TestSubmitDeniedByUserPromptSubmitHookShowsReasonAndStartsNoTurn(t *testing.T) {
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: `echo "no chatting about that" >&2; exit 1`}},
	}}
	m := New(agent.Loop{Hooks: hooks}, config.Config{}, nil)
	m.input.SetValue("hello there")

	next, cmd := m.submit()
	mm := next.(Model)

	if cmd != nil {
		t.Error("submit() returned a non-nil cmd, want nil (a denied prompt starts no turn)")
	}
	if mm.busy {
		t.Error("busy = true, want false (a denied prompt never starts a turn)")
	}
	if len(mm.lines) != 2 {
		t.Fatalf("lines = %+v, want [user text, system denial]", mm.lines)
	}
	if mm.lines[0].role != "user" || mm.lines[0].text != "hello there" {
		t.Errorf("lines[0] = %+v, want the user's own typed text", mm.lines[0])
	}
	if mm.lines[1].role != "system" || !strings.Contains(mm.lines[1].text, "no chatting about that") {
		t.Errorf("lines[1] = %+v, want a system line mentioning the hook's stderr", mm.lines[1])
	}
}

// TestSubmitAllowedByUserPromptSubmitHookProceedsNormally covers the
// complementary case: a userPromptSubmit hook that exits 0 lets the
// submission proceed through the reserved-command dispatch table exactly as
// if no hook were configured.
func TestSubmitAllowedByUserPromptSubmitHookProceedsNormally(t *testing.T) {
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: "exit 0"}},
	}}
	m := New(agent.Loop{Hooks: hooks}, config.Config{}, nil)
	m.input.SetValue("/skills")

	next, cmd := m.submit()
	mm := next.(Model)

	if cmd != nil {
		t.Error("submit(\"/skills\") returned a non-nil cmd, want nil (no turn started)")
	}
	if len(mm.lines) != 1 || mm.lines[0].role != "raw" {
		t.Fatalf("lines = %+v, want the /skills dispatch to still run normally", mm.lines)
	}
}

func TestSubmitSlashSkillsRendersCatalogAsRawLine(t *testing.T) {
	skills := []skill.Skill{
		{Name: "commit-messages", Description: "Write conventional commit messages.", Scope: skill.ScopeUser},
	}
	m := New(agent.Loop{}, config.Config{}, skills)
	m.input.SetValue("/skills")

	next, cmd := m.submit()
	mm := next.(Model)

	if cmd != nil {
		t.Error("submit(\"/skills\") returned a non-nil cmd, want nil (no turn started)")
	}
	if len(mm.lines) != 1 || mm.lines[0].role != "raw" {
		t.Fatalf("lines = %+v, want a single raw line", mm.lines)
	}
	if got := ansi.Strip(mm.lines[0].text); !strings.Contains(got, "commit-messages") {
		t.Errorf("lines[0].text = %q, want it to mention the discovered skill", got)
	}
}

func TestSubmitSlashSkillsWithNoneDiscovered(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.input.SetValue("/skills")

	next, _ := m.submit()
	mm := next.(Model)

	if len(mm.lines) != 1 || ansi.Strip(mm.lines[0].text) != "No skills discovered." {
		t.Fatalf("lines = %+v, want a single \"No skills discovered.\" raw line", mm.lines)
	}
}

func TestSubmitIgnoresEmptyInputAndInputWhileBusy(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)

	m.input.SetValue("   ")
	if _, cmd := m.submit(); cmd != nil {
		t.Error("submit() on blank input returned a non-nil cmd")
	}

	m.busy = true
	m.input.SetValue("hello")
	next, cmd := m.submit()
	if cmd != nil {
		t.Error("submit() while busy returned a non-nil cmd, want a no-op")
	}
	if len(next.(Model).lines) != 0 {
		t.Error("submit() while busy appended a line, want no-op")
	}
}

func TestCancelTurnCancelsContextWhenBusy(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	canceled := false
	m.busy = true
	m.cancel = func() { canceled = true }

	m.cancelTurn()

	if !canceled {
		t.Error("cancelTurn() did not call the stored cancel func while busy")
	}
}

func TestCancelTurnNoOpWhenIdle(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	called := false
	m.cancel = func() { called = true }

	m.cancelTurn()

	if called {
		t.Error("cancelTurn() called cancel() while not busy")
	}
}

func TestFinishTurnMarksInterruptedAndPreservesPartialOutput(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.busy = true
	m.streaming.WriteString("partial ans")
	partialMessages := []provider.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "partial ans"},
	}

	m.finishTurn(partialMessages, context.Canceled)

	if m.busy {
		t.Error("busy = true after finishTurn, want false")
	}
	if len(m.lines) != 2 {
		t.Fatalf("lines = %+v, want 2 (flushed partial text + [interrupted])", m.lines)
	}
	if m.lines[0].role != "assistant" || m.lines[0].text != "partial ans" {
		t.Errorf("lines[0] = %+v, want the flushed partial assistant text", m.lines[0])
	}
	if m.lines[1].role != "system" || m.lines[1].text != "[interrupted]" {
		t.Errorf("lines[1] = %+v, want the [interrupted] marker", m.lines[1])
	}
	if len(m.sess.Messages) != 2 {
		t.Errorf("sess.Messages = %+v, want the partial history preserved", m.sess.Messages)
	}
}

func TestFinishTurnMarksErrorForNonCancelFailures(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.busy = true

	m.finishTurn(nil, errors.New("boom"))

	if len(m.lines) != 1 || m.lines[0].role != "system" || m.lines[0].text != "[error: boom]" {
		t.Errorf("lines = %+v, want a single [error: boom] system line", m.lines)
	}
}

// completionLinePattern matches completionLine's "✓ Completed @ <HH:MM>
// after <elapsed>" shape (issue #166, 24-hour clock) regardless of the
// actual wall-clock time or elapsed duration, both of which vary with the
// moment the test runs.
var completionLinePattern = regexp.MustCompile(`^✓ Completed @ \d{2}:\d{2} after \S+$`)

func TestFinishTurnAppendsCompletionLineOnSuccess(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.busy = true
	m.turnStart = time.Now().Add(-1 * time.Second)

	m.finishTurn([]provider.Message{{Role: "user", Content: "hi"}}, nil)

	if len(m.lines) != 1 {
		t.Fatalf("lines = %+v, want a single completion line", m.lines)
	}
	got := m.lines[0]
	if got.role != "complete" {
		t.Errorf("lines[0].role = %q, want %q", got.role, "complete")
	}
	if !completionLinePattern.MatchString(got.text) {
		t.Errorf("lines[0].text = %q, want it to match %s", got.text, completionLinePattern)
	}
}

func TestUpdateBackgroundColorMsgResolvesTheme(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil)

	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	mm := next.(Model)

	if mm.pal.Dark {
		t.Errorf("pal = %+v, want the light palette for a light BackgroundColorMsg", mm.pal)
	}
}

// isBackgroundColorRequestMsg reports whether msg is the private sentinel
// tea.RequestBackgroundColor() itself produces — the type is unexported, so
// this can't be a type switch from outside package tea; comparing the
// reflect type name is the only option available to a test in this package.
func isBackgroundColorRequestMsg(msg tea.Msg) bool {
	return reflect.TypeOf(msg).String() == "tea.backgroundColorMsg"
}

// batchIncludesBackgroundColorRequest runs every Cmd in batch and reports
// whether any of them produced tea.RequestBackgroundColor()'s own sentinel
// Msg — used to confirm Update's FocusMsg case's batched re-query (wrapped
// in a tea.Tick, unlike Init's direct tea.RequestBackgroundColor) actually
// includes a real OSC-11 GET request once its delay elapses.
func batchIncludesBackgroundColorRequest(batch tea.BatchMsg) bool {
	for _, c := range batch {
		if isBackgroundColorRequestMsg(c()) {
			return true
		}
	}
	return false
}

// TestViewWithholdsBackgroundColorWhileThemeDetectPending guards the actual
// root cause behind #103's live re-detection silently never working: a
// terminal's OSC-11 GET simply echoes back whatever was most recently SET,
// so painting v.BackgroundColor from a stale/placeholder m.pal while a
// detection reply is in flight — at startup, or after a later FocusMsg
// re-query — would make the query's own reply just echo that same stale
// value back, permanently poisoning detection (confirmed by direct OSC-11
// set-then-get testing; see docs/adr/0010's addendum).
func TestViewWithholdsBackgroundColorWhileThemeDetectPending(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil).WithCwd("/cwd")
	m.width, m.height = 80, 24

	if !m.themeDetectPending {
		t.Fatal("themeDetectPending = false right after New() in auto mode, want true (test's own premise)")
	}
	if got := m.View().BackgroundColor; got != nil {
		t.Errorf("View().BackgroundColor = %v while themeDetectPending, want nil so it can't poison the in-flight OSC-11 query", got)
	}

	// The startup query's reply arrives — painting should resume, using
	// the newly resolved palette.
	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	m = next.(Model)
	if m.themeDetectPending {
		t.Fatal("themeDetectPending = true after BackgroundColorMsg, want false (test's own premise)")
	}
	if got := m.View().BackgroundColor; got == nil {
		t.Error("View().BackgroundColor = nil once detection resolved, want the resolved palette's Base painted again")
	}

	// A later focus event re-issuing the query must withhold painting
	// again, exactly like startup — this is the actual re-detection fix:
	// without it, the re-query's own reply would just read back this
	// Model's own last-painted color instead of the terminal's real one.
	next, _ = m.Update(tea.FocusMsg{})
	m = next.(Model)
	if !m.themeDetectPending {
		t.Fatal("themeDetectPending = false right after FocusMsg, want true (test's own premise)")
	}
	if got := m.View().BackgroundColor; got != nil {
		t.Errorf("View().BackgroundColor = %v while a focus-triggered re-query is pending, want nil", got)
	}
}

// TestThemeDetectTimeoutMsgResumesPaintingWithoutChangingPalette covers the
// fallback when a terminal never answers the OSC-11 query at all (e.g.
// tmux/screen's blanket refusal, or a focus-triggered re-query that never
// gets a reply): themeDetectPending must eventually clear on its own so
// View() doesn't withhold v.BackgroundColor forever, and it must do so
// without altering m.pal — there's no reply to resolve a new palette from,
// so the prior (already-correct) palette's own paint should simply resume.
func TestThemeDetectTimeoutMsgResumesPaintingWithoutChangingPalette(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil).WithCwd("/cwd")
	m.width, m.height = 80, 24
	wantPal := m.pal

	next, _ := m.Update(themeDetectTimeoutMsg{})
	m = next.(Model)

	if m.themeDetectPending {
		t.Error("themeDetectPending = true after themeDetectTimeoutMsg, want false")
	}
	if m.pal != wantPal {
		t.Errorf("pal = %+v after themeDetectTimeoutMsg, want unchanged %+v (no reply to resolve a new one from)", m.pal, wantPal)
	}
	if got := m.View().BackgroundColor; got == nil {
		t.Error("View().BackgroundColor = nil after themeDetectTimeoutMsg, want painting to resume")
	}
}

func TestUpdateFocusMsgReRequestsBackgroundColorInAutoMode(t *testing.T) {
	// "auto" and "" (unset) are equivalent — both are the documented
	// default (config.ThemeConfig.Mode's own doc comment) — so both must
	// trigger the same focus-driven re-query.
	for _, mode := range []string{"auto", ""} {
		m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: mode}}, nil)
		// Avoid the real themeRequeryDelay/bannerTimeout sleeps when the
		// test invokes the returned Cmd below (matches
		// TestNewAppliesThemeModeOverrideWithoutDetection's own approach).
		m.themeRequeryDelay = 0
		m.bannerTimeout = 0

		_, cmd := m.Update(tea.FocusMsg{})
		if cmd == nil {
			t.Fatalf("theme.mode=%q: Update(FocusMsg) cmd = nil, want a batched re-query", mode)
		}
		batch, ok := cmd().(tea.BatchMsg)
		if !ok {
			t.Fatalf("theme.mode=%q: Update(FocusMsg) cmd() = %#v, want tea.BatchMsg (re-query + detect-timeout)", mode, cmd())
		}
		if !batchIncludesBackgroundColorRequest(batch) {
			t.Errorf("theme.mode=%q: Update(FocusMsg)'s batch didn't include a background-color re-query", mode)
		}
	}
}

func TestViewReportsFocusOnlyInAutoMode(t *testing.T) {
	for mode, want := range map[string]bool{"auto": true, "": true, "dark": false, "light": false} {
		m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: mode}}, nil).WithCwd("/cwd")
		m.width, m.height = 80, 24

		if got := m.View().ReportFocus; got != want {
			t.Errorf("theme.mode=%q: View().ReportFocus = %v, want %v", mode, got, want)
		}
	}
}

func TestUpdateFocusMsgNoOpInExplicitThemeMode(t *testing.T) {
	for _, mode := range []string{"dark", "light"} {
		m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: mode}}, nil)

		_, cmd := m.Update(tea.FocusMsg{})
		if cmd != nil {
			t.Errorf("theme.mode=%q: Update(FocusMsg) cmd = %#v, want nil (no re-query once the user has opted out via an explicit override)", mode, cmd)
		}
	}
}

func TestFocusTriggeredReQueryChangesActiveTheme(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil)
	// Avoid the real themeRequeryDelay/bannerTimeout sleeps when the test
	// invokes the returned Cmd below.
	m.themeRequeryDelay = 0
	m.bannerTimeout = 0
	if !m.pal.Dark {
		t.Fatal("pal.Dark = false before any detection, want the dark-assumed default (test's own premise)")
	}

	next, cmd := m.Update(tea.FocusMsg{})
	m = next.(Model)
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || !batchIncludesBackgroundColorRequest(batch) {
		t.Fatalf("Update(FocusMsg) cmd() = %#v, want a batch including a background-color re-query", cmd())
	}
	if !m.themeDetectPending {
		t.Error("themeDetectPending = false right after FocusMsg, want true until the reply arrives")
	}

	// Simulate the terminal's reply to that re-issued query — a separate
	// message from the request sentinel cmd() would itself produce, exactly
	// as it would arrive asynchronously from a real terminal — as if the
	// user's OS/terminal theme changed while liam was unfocused.
	next, _ = m.Update(tea.BackgroundColorMsg{Color: color.White})
	mm := next.(Model)
	if mm.pal.Dark {
		t.Errorf("pal = %+v after a light BackgroundColorMsg following a focus event, want the light palette to take effect without a restart", mm.pal)
	}
	if mm.themeDetectPending {
		t.Error("themeDetectPending = true after BackgroundColorMsg, want false so View() resumes painting")
	}
}

// batchIncludesRaw reports whether any Cmd in batch produces a
// tea.RawMsg wrapping want — the shape tea.Raw(...) commands resolve to
// (issue #203, docs/adr/0018).
func batchIncludesRaw(batch tea.BatchMsg, want string) bool {
	for _, c := range batch {
		if raw, ok := c().(tea.RawMsg); ok && raw.Msg == want {
			return true
		}
	}
	return false
}

// sequenceSteps extracts a tea.Sequence(...)'s underlying []tea.Cmd via
// reflection: Bubbletea's sequenceMsg is an unexported named type
// (`type sequenceMsg []Cmd`), so a test outside package tea can't
// type-assert against it directly — but its element type, tea.Cmd, is
// exported, so reflect.Value.Index(i).Interface() still yields a usable
// tea.Cmd (mirrors isBackgroundColorRequestMsg's reflect-based sentinel
// check above). Used by quitCmd's tests to confirm strict ordering
// (mode-2031 disable, then tea.Quit) — the entire reason quitCmd uses
// tea.Sequence instead of tea.Batch (see its own doc comment).
func sequenceSteps(msg tea.Msg) ([]tea.Cmd, bool) {
	if reflect.TypeOf(msg).String() != "tea.sequenceMsg" {
		return nil, false
	}
	v := reflect.ValueOf(msg)
	steps := make([]tea.Cmd, v.Len())
	for i := range steps {
		steps[i] = v.Index(i).Interface().(tea.Cmd)
	}
	return steps, true
}

// TestInitEnablesColorSchemePushInAutoModeOnly covers issue #203/docs/adr/
// 0018's second, independent live re-detection path: Init sends the DEC
// mode 2031 enable sequence once per session, gated identically to the
// existing OSC-11 startup query — only under theme.mode "auto" (including
// unset/""), never under an explicit dark/light override.
func TestInitEnablesColorSchemePushInAutoModeOnly(t *testing.T) {
	for mode, wantEnabled := range map[string]bool{"auto": true, "": true, "dark": false, "light": false} {
		noTimer := 0
		m := New(agent.Loop{}, config.Config{
			Theme:      config.ThemeConfig{Mode: mode},
			StatusLine: config.StatusLineConfig{RefreshInterval: &noTimer}, // avoid a real 1s sleep when the test invokes cmd() below
		}, nil)
		m.statusDebounce = 0    // avoid a real 300ms sleep
		m.bannerTimeout = 0     // avoid a real 300ms sleep (auto mode's bannerTimeoutCmd/themeDetectTimeoutCmd)
		m.themeRequeryDelay = 0 // unused here, matches other Init tests' belt-and-suspenders overrides

		cmd := m.Init()
		if cmd == nil {
			t.Fatalf("theme.mode=%q: Init() = nil", mode)
		}
		batch, ok := cmd().(tea.BatchMsg)
		if !ok {
			t.Fatalf("theme.mode=%q: Init() = %#v, want tea.BatchMsg", mode, cmd())
		}
		if got := batchIncludesRaw(batch, ansi.SetModeLightDark); got != wantEnabled {
			t.Errorf("theme.mode=%q: Init()'s batch includes the mode-2031 enable sequence = %v, want %v", mode, got, wantEnabled)
		}
	}
}

// TestUpdateColorSchemePushAppliesPaletteInAutoMode covers the actual
// re-detection: unlike tea.BackgroundColorMsg's OSC-11 reply, a DEC mode
// 2031 push (issue #203, docs/adr/0018) carries its own definitive
// dark/light answer directly, so applying it needs no follow-up Cmd and no
// themeDetectPending withholding.
func TestUpdateColorSchemePushAppliesPaletteInAutoMode(t *testing.T) {
	tests := []struct {
		name     string
		event    tea.Msg
		wantDark bool
	}{
		{"dark push", uv.DarkColorSchemeEvent{}, true},
		{"light push", uv.LightColorSchemeEvent{}, false},
	}
	for _, tt := range tests {
		m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil)

		next, cmd := m.Update(tt.event)
		mm := next.(Model)
		if cmd != nil {
			t.Errorf("%s: Update cmd = %#v, want nil (a direct push needs no follow-up query)", tt.name, cmd)
		}
		if mm.pal.Dark != tt.wantDark {
			t.Errorf("%s: pal.Dark = %v, want %v", tt.name, mm.pal.Dark, tt.wantDark)
		}
	}
}

// TestColorSchemePushClearsInFlightDetectPending covers a gap found while
// diagnosing issue #203 in the field: a mode-2031 push arriving while an
// unrelated focus-triggered OSC-11 re-query (issue #103) is still in
// flight must resume painting immediately rather than staying withheld
// until that stale query's reply arrives. The push already carries a
// trustworthy, definitive answer — there's no reason for it to wait on an
// older round trip it didn't start; when that stale reply eventually
// arrives, it just re-confirms whatever the push already set (a terminal's
// OSC-11 GET echoes back the most recently painted color, so no harm, no
// poisoning — see themeDetectPending's doc comment on Model).
func TestColorSchemePushClearsInFlightDetectPending(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil).WithCwd("/cwd")
	m.width, m.height = 80, 24
	m.themeRequeryDelay = 0
	m.bannerTimeout = 0

	// Resolve initial detection, then a focus event puts a re-query in
	// flight (themeDetectPending withholds View().BackgroundColor).
	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.Black})
	m = next.(Model)
	next, _ = m.Update(tea.FocusMsg{})
	m = next.(Model)
	if !m.themeDetectPending {
		t.Fatal("themeDetectPending = false right after FocusMsg, want true (test's own premise)")
	}

	// The mode-2031 push arrives before that re-query's own reply does.
	next, _ = m.Update(uv.LightColorSchemeEvent{})
	m = next.(Model)

	if m.themeDetectPending {
		t.Error("themeDetectPending = true after a mode-2031 push, want false: the push already answered definitively, so painting should resume immediately rather than wait on the stale in-flight re-query")
	}
	if got := m.View().BackgroundColor; got == nil {
		t.Error("View().BackgroundColor = nil after a mode-2031 push resolved during an in-flight re-query, want painting to resume with the push's palette")
	}
}

// TestUpdateColorSchemePushNoOpInExplicitThemeMode mirrors
// TestUpdateFocusMsgNoOpInExplicitThemeMode for the mode-2031 push path: an
// explicit dark/light override means the user opted out of detection
// entirely, so even a received push (which shouldn't happen in practice,
// since enabling the push in the first place is gated the same way) must
// leave the palette untouched.
func TestUpdateColorSchemePushNoOpInExplicitThemeMode(t *testing.T) {
	for _, mode := range []string{"dark", "light"} {
		for _, event := range []tea.Msg{uv.DarkColorSchemeEvent{}, uv.LightColorSchemeEvent{}} {
			m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: mode}}, nil)
			wantPal := m.pal

			next, cmd := m.Update(event)
			mm := next.(Model)
			if cmd != nil {
				t.Errorf("theme.mode=%q: Update(%T) cmd = %#v, want nil", mode, event, cmd)
			}
			if mm.pal != wantPal {
				t.Errorf("theme.mode=%q: pal = %+v after %T, want unchanged %+v (opted out via explicit override)", mode, mm.pal, event, wantPal)
			}
		}
	}
}

// TestQuitCmdDisablesColorSchemePushInAutoMode covers quitCmd's teardown
// half of issue #203/docs/adr/0018: tea.Raw's enable sequence gets no
// automatic reset-on-exit from Bubbletea's renderer (unlike a View()
// field), so liam must disable mode 2031 itself on quit — strictly before
// tea.Quit, not merely alongside it (see quitCmd's own doc comment for why
// a tea.Batch's no-ordering-guarantee would let this race and silently
// drop the disable write).
func TestQuitCmdDisablesColorSchemePushInAutoMode(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil)

	cmd := m.quitCmd()
	steps, ok := sequenceSteps(cmd())
	if !ok {
		t.Fatalf("quitCmd() = %#v, want a tea.Sequence (mode-2031 disable, then tea.Quit)", cmd())
	}
	if len(steps) != 2 {
		t.Fatalf("quitCmd()'s sequence has %d steps, want 2 (disable, then quit)", len(steps))
	}
	if raw, ok := steps[0]().(tea.RawMsg); !ok || raw.Msg != ansi.ResetModeLightDark {
		t.Errorf("quitCmd()'s first step produced %#v, want the mode-2031 disable sequence", steps[0]())
	}
	if _, ok := steps[1]().(tea.QuitMsg); !ok {
		t.Errorf("quitCmd()'s second step produced %#v, want tea.QuitMsg", steps[1]())
	}
}

// TestQuitCmdOmitsColorSchemeDisableInExplicitThemeMode covers the opt-out
// side: an explicit dark/light override means mode 2031 was never enabled
// (TestInitEnablesColorSchemePushInAutoModeOnly), so there's nothing to
// disable on quit — quitCmd should return tea.Quit directly, not a batch.
func TestQuitCmdOmitsColorSchemeDisableInExplicitThemeMode(t *testing.T) {
	for _, mode := range []string{"dark", "light"} {
		m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: mode}}, nil)

		cmd := m.quitCmd()
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("theme.mode=%q: quitCmd()() = %#v, want tea.QuitMsg directly (nothing to disable)", mode, cmd())
		}
	}
}

func TestNewAppliesThemeModeOverrideWithoutDetection(t *testing.T) {
	noTimer := 0
	m := New(agent.Loop{}, config.Config{
		Theme:      config.ThemeConfig{Mode: "light"},
		StatusLine: config.StatusLineConfig{RefreshInterval: &noTimer}, // avoid a real 1s sleep when the test invokes cmd() below
	}, nil)
	m.statusDebounce = 0 // avoid a real 300ms sleep when the test invokes cmd() below
	if m.pal.Dark {
		t.Errorf("pal = %+v, want the light palette when theme.mode=light", m.pal)
	}

	// Init() always fires the statusLine session-start refresh (issue #60)
	// and the startup banner (issue #169), so it's never nil; theme.mode=
	// light's own effect is that no background-color request is batched
	// alongside them.
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() = nil, want the statusLine session-start refresh and the startup banner")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() = %#v, want a tea.BatchMsg (statusTick + showBanner, no background-color request)", cmd())
	}

	var sawStatus, sawBanner bool
	for _, c := range batch {
		switch c().(type) {
		case statusRefreshMsg:
			sawStatus = true
		case bannerMsg:
			sawBanner = true
		default:
			t.Errorf("Init() batched an unexpected cmd producing %#v despite theme.mode override", c())
		}
	}
	if !sawStatus {
		t.Error("Init() didn't include the statusLine session-start refresh")
	}
	if !sawBanner {
		t.Error("Init() didn't include the startup banner")
	}
}

// fakeTool records the args it was called with and returns a scripted
// Result, mirroring internal/agent's own test double.
type fakeTool struct {
	name   string
	result tool.Result
}

func (f *fakeTool) Name() string            { return f.name }
func (f *fakeTool) Description() string     { return "fake tool" }
func (f *fakeTool) Parameters() tool.Schema { return tool.Schema{"type": "object"} }
func (f *fakeTool) Safety() tool.Safety {
	return tool.Safety{SideEffect: tool.SideEffectRead}
}
func (f *fakeTool) Run(context.Context, map[string]any) tool.Result { return f.result }
