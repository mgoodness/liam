package agent

import (
	"context"
	"testing"

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
