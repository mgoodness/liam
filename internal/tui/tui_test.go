package tui

import (
	"context"
	"errors"
	"image/color"
	"iter"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
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

// drain repeatedly invokes cmd and feeds the resulting Msg back through
// Update, exactly as a real tea.Program would, until a turnDoneMsg is
// processed. It returns the final Model.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("drain: nil cmd, nothing to pump")
	}
	for {
		msg := cmd()
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
		if _, ok := msg.(turnDoneMsg); ok {
			return m
		}
	}
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
	if len(final.lines) != 2 {
		t.Fatalf("lines after turn = %+v, want 2 (user + assistant)", final.lines)
	}
	if final.lines[1].role != "assistant" || final.lines[1].text != "hello" {
		t.Errorf("lines[1] = %+v, want assistant %q", final.lines[1], "hello")
	}
	if final.sess.CostUSD != 0.01 {
		t.Errorf("sess.CostUSD = %v, want 0.01", final.sess.CostUSD)
	}
	if len(final.sess.Messages) != 2 {
		t.Fatalf("sess.Messages = %+v, want 2 (user + assistant)", final.sess.Messages)
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
	m.input.SetValue("read foo")

	next, cmd := m.submit()
	final := drain(t, next.(Model), cmd)

	// user, "looking" (flushed before the tool call), tool result, "done"
	if len(final.lines) != 4 {
		t.Fatalf("lines = %+v, want 4", final.lines)
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
}

func TestSubmitSlashQuitReturnsQuitCmd(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.input.SetValue("/quit")

	_, cmd := m.submit()
	if cmd == nil {
		t.Fatal("submit(\"/quit\") returned a nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("submit(\"/quit\") cmd produced %#v, want tea.QuitMsg", cmd())
	}
}

func TestSubmitSlashClearResetsSessionAndLines(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.lines = []line{{role: "user", text: "old"}}
	m.sess.Messages = []provider.Message{{Role: "user", Content: "old"}}
	oldID := m.sess.ID
	m.input.SetValue("/clear")

	next, cmd := m.submit()
	mm := next.(Model)

	if cmd != nil {
		t.Error("submit(\"/clear\") returned a non-nil cmd, want nil (no turn started)")
	}
	if len(mm.lines) != 0 {
		t.Errorf("lines = %+v, want empty after /clear", mm.lines)
	}
	if mm.sess.Messages != nil {
		t.Errorf("sess.Messages = %+v, want nil after /clear", mm.sess.Messages)
	}
	if mm.sess.ID == oldID {
		t.Error("/clear did not assign a fresh session ID")
	}
}

func TestSubmitSlashSkillsRendersCatalogAsInfoLine(t *testing.T) {
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
	if len(mm.lines) != 1 || mm.lines[0].role != "info" {
		t.Fatalf("lines = %+v, want a single info line", mm.lines)
	}
	if !strings.Contains(mm.lines[0].text, "commit-messages") {
		t.Errorf("lines[0].text = %q, want it to mention the discovered skill", mm.lines[0].text)
	}
}

func TestSubmitSlashSkillsWithNoneDiscovered(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.input.SetValue("/skills")

	next, _ := m.submit()
	mm := next.(Model)

	if len(mm.lines) != 1 || mm.lines[0].text != "No skills discovered." {
		t.Fatalf("lines = %+v, want a single \"No skills discovered.\" info line", mm.lines)
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

func TestUpdateBackgroundColorMsgResolvesTheme(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "auto"}}, nil)

	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	mm := next.(Model)

	if mm.pal.Dark {
		t.Errorf("pal = %+v, want the light palette for a light BackgroundColorMsg", mm.pal)
	}
}

func TestNewAppliesThemeModeOverrideWithoutDetection(t *testing.T) {
	m := New(agent.Loop{}, config.Config{Theme: config.ThemeConfig{Mode: "light"}}, nil)
	if m.pal.Dark {
		t.Errorf("pal = %+v, want the light palette when theme.mode=light", m.pal)
	}
	if cmd := m.Init(); cmd != nil {
		t.Error("Init() returned a background-color request despite theme.mode override")
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
