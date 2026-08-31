package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
)

// fakeMCPLoader implements mcpLoader without a real mcp.Loader, so tests
// can script its results and count calls to verify the "waited for on the
// first turn only" contract.
type fakeMCPLoader struct {
	tools    []tool.Tool
	timedOut bool
	errs     map[string]error
	calls    int
}

func (f *fakeMCPLoader) Tools(context.Context, time.Duration) ([]tool.Tool, bool) {
	f.calls++
	return f.tools, f.timedOut
}
func (f *fakeMCPLoader) Errs() map[string]error { return f.errs }

// TestSubmitMergesMCPToolsBeforeFirstTurnDispatch covers issue #48's
// "registered into liam's toolset as if they were built-in Tools" reaching
// the running TUI: a loader-reported tool must be callable on the very
// first turn, proving the merge happens before dispatch, not just
// registration for its own sake.
func TestSubmitMergesMCPToolsBeforeFirstTurnDispatch(t *testing.T) {
	ft := &fakeTool{name: "mcp_tool", result: tool.Result{Content: "mcp result"}}
	loader := &fakeMCPLoader{tools: []tool.Tool{ft}}
	fp := &multiCallProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "mcp_tool", ArgsJSON: `{}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	m := New(agent.Loop{Provider: fp, Tools: tool.NewRegistry()}, config.Config{}, nil).WithMCPLoader(loader)
	m.input.SetValue("use the mcp tool")

	next, cmd := m.submit()
	final := drain(t, next.(Model), cmd)

	var sawResult bool
	for _, l := range final.lines {
		if l.role == "tool" && strings.Contains(l.text, "mcp result") {
			sawResult = true
		}
	}
	if !sawResult {
		t.Errorf("lines = %+v, want a tool line containing the mcp tool's result", final.lines)
	}
}

// TestSubmitWaitsForMCPOnFirstTurnOnlyNotSubsequent covers "the first
// actual model call blocks... after which it proceeds" — the loader must
// be waited on exactly once per session, not on every turn.
func TestSubmitWaitsForMCPOnFirstTurnOnlyNotSubsequent(t *testing.T) {
	loader := &fakeMCPLoader{}
	fp := &multiCallProvider{turns: [][]provider.Event{
		{provider.DoneEvent{FinishReason: "stop"}},
		{provider.DoneEvent{FinishReason: "stop"}},
	}}
	m := New(agent.Loop{Provider: fp, Tools: tool.NewRegistry()}, config.Config{}, nil).WithMCPLoader(loader)

	m.input.SetValue("first")
	next, cmd := m.submit()
	mm := drain(t, next.(Model), cmd)
	if loader.calls != 1 {
		t.Fatalf("loader.Tools called %d times after first turn, want 1", loader.calls)
	}

	mm.input.SetValue("second")
	next2, cmd2 := mm.submit()
	drain(t, next2.(Model), cmd2)
	if loader.calls != 1 {
		t.Errorf("loader.Tools called %d times after second turn, want still 1 (first-turn-only)", loader.calls)
	}
}

// TestSubmitRendersSystemLineOnMCPLoadTimeout covers "logging a warning on
// timeout" reaching the TUI's own scrollback (its equivalent of a log).
func TestSubmitRendersSystemLineOnMCPLoadTimeout(t *testing.T) {
	loader := &fakeMCPLoader{timedOut: true}
	fp := &multiCallProvider{turns: [][]provider.Event{{provider.DoneEvent{FinishReason: "stop"}}}}
	m := New(agent.Loop{Provider: fp, Tools: tool.NewRegistry()}, config.Config{}, nil).WithMCPLoader(loader)
	m.input.SetValue("hi")

	next, cmd := m.submit()
	final := drain(t, next.(Model), cmd)

	var sawWarning bool
	for _, l := range final.lines {
		if l.role == "system" && strings.Contains(l.text, "timed out") {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Errorf("lines = %+v, want a system line mentioning the MCP load timeout", final.lines)
	}
}

// TestSubmitRendersSystemLineOnMCPServerError covers a per-server load
// failure (independent of timeout) reaching the scrollback.
func TestSubmitRendersSystemLineOnMCPServerError(t *testing.T) {
	loader := &fakeMCPLoader{errs: map[string]error{"bad-server": errors.New("connect refused")}}
	fp := &multiCallProvider{turns: [][]provider.Event{{provider.DoneEvent{FinishReason: "stop"}}}}
	m := New(agent.Loop{Provider: fp, Tools: tool.NewRegistry()}, config.Config{}, nil).WithMCPLoader(loader)
	m.input.SetValue("hi")

	next, cmd := m.submit()
	final := drain(t, next.(Model), cmd)

	var sawWarning bool
	for _, l := range final.lines {
		if l.role == "system" && strings.Contains(l.text, "bad-server") {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Errorf("lines = %+v, want a system line mentioning the failing server", final.lines)
	}
}
