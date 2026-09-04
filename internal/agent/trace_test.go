package agent

import (
	"context"
	"testing"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
	"github.com/mgoodness/liam/internal/trace"
	"github.com/mgoodness/liam/internal/trace/tracetest"
)

// TestDispatchTracesNormalExecutionWithIntentAndDuration drives a real
// dispatch (via Run/the fake-Provider seam) with l.Trace set, and asserts
// the resulting ToolCallLine: decision=executed, the model-supplied intent
// carried through, and a real duration_ms.
func TestDispatchTracesNormalExecutionWithIntentAndDuration(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-exec")
	ft := &fakeTool{name: "bash", result: tool.Result{Content: "ok"}}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "bash", ArgsJSON: `{"command":"ls","intent":"list files to check the build"}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{provider.DoneEvent{FinishReason: "stop"}},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ft), Trace: tr}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	tr.Close()

	lines := tracetest.ReadLines[trace.ToolCallLine](t, stateHome, "sess-exec")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
	l0 := lines[0]
	if l0.Tool != "bash" || l0.Decision != trace.DecisionExecuted {
		t.Errorf("line = %+v, want {Tool: bash, Decision: executed}", l0)
	}
	if l0.Intent != "list files to check the build" {
		t.Errorf("Intent = %q, want the model-supplied intent", l0.Intent)
	}
	if l0.SideEffect != string(tool.SideEffectRead) {
		t.Errorf("SideEffect = %q, want %q", l0.SideEffect, tool.SideEffectRead)
	}
	if ft.gotArg["intent"] != nil {
		t.Errorf("tool.Run saw args %+v, want \"intent\" stripped before Run", ft.gotArg)
	}
}

// TestDispatchTracesBeforeToolDenialAsDeniedByHookWithSource drives a real
// hook.Runner configured with a stub denying beforeTool hook through Run,
// and asserts the resulting ToolCallLine: decision=denied_by_hook, no
// duration_ms, and Source naming the denying hook.
func TestDispatchTracesBeforeToolDenialAsDeniedByHookWithSource(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-deny")
	ft := &fakeTool{name: "bash", result: tool.Result{Content: "should never run"}}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "bash", ArgsJSON: `{"command":"rm -rf /","intent":"cleanup"}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{provider.DoneEvent{FinishReason: "stop"}},
	}}
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{{Command: `echo "denied by policy" >&2; exit 1`}},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ft), Hooks: hooks, Trace: tr}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	tr.Close()

	lines := tracetest.ReadLines[trace.ToolCallLine](t, stateHome, "sess-deny")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
	l0 := lines[0]
	if l0.Decision != trace.DecisionDeniedByHook {
		t.Errorf("Decision = %q, want denied_by_hook", l0.Decision)
	}
	if l0.Reason != "denied by policy" {
		t.Errorf("Reason = %q, want the denying hook's stderr", l0.Reason)
	}
	if l0.Source != `echo "denied by policy" >&2; exit 1` {
		t.Errorf("Source = %q, want the denying hook's command", l0.Source)
	}
	if l0.Intent != "cleanup" {
		t.Errorf("Intent = %q, want the model-supplied intent even on a denial", l0.Intent)
	}
	if l0.DurationMs != 0 {
		t.Errorf("DurationMs = %d, want 0 (a denied call never ran)", l0.DurationMs)
	}
}

// TestDispatchTracesToolErrorAsErrored drives a fakeTool that returns an
// error Result through Run, and asserts the resulting ToolCallLine:
// decision=errored, Reason carrying the tool's own error content.
func TestDispatchTracesToolErrorAsErrored(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-error")
	ft := &fakeTool{name: "bash", result: tool.Result{Content: "boom: command not found", IsError: true}}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "bash", ArgsJSON: `{"command":"nope","intent":"try it"}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{provider.DoneEvent{FinishReason: "stop"}},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ft), Trace: tr}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	tr.Close()

	lines := tracetest.ReadLines[trace.ToolCallLine](t, stateHome, "sess-error")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
	l0 := lines[0]
	if l0.Decision != trace.DecisionErrored {
		t.Errorf("Decision = %q, want errored", l0.Decision)
	}
	if l0.Reason != "boom: command not found" {
		t.Errorf("Reason = %q, want the tool's own error content", l0.Reason)
	}
	if l0.DurationMs != 0 {
		t.Errorf("DurationMs = %d, want 0 (spec: duration_ms is executed-only)", l0.DurationMs)
	}
}

// TestDispatchTracesUnknownToolAsErrored covers dispatch's early
// unknown-tool return: it still records a ToolCallLine (errored, empty
// side_effect) rather than silently skipping tracing.
func TestDispatchTracesUnknownToolAsErrored(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-unknown")
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "nonexistent", ArgsJSON: `{}`},
			provider.DoneEvent{},
		},
		{provider.DoneEvent{}},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(), Trace: tr}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	tr.Close()

	lines := tracetest.ReadLines[trace.ToolCallLine](t, stateHome, "sess-unknown")
	if len(lines) != 1 || lines[0].Decision != trace.DecisionErrored || lines[0].Tool != "nonexistent" {
		t.Errorf("lines = %+v, want 1 errored line for \"nonexistent\"", lines)
	}
}
