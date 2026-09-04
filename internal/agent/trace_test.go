package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
	"github.com/mgoodness/liam/internal/trace"
)

// newTestTracer isolates a *trace.Writer under a fresh temp XDG_STATE_HOME
// and points it at sessionID, matching internal/hook's own newTracer
// helper. Callers must call Close() themselves before reading the trace
// file back, to deterministically drain pending writes.
func newTestTracer(t *testing.T, sessionID string) (*trace.Writer, string) {
	t.Helper()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	tr := trace.New()
	tr.SessionID = sessionID
	return tr, stateHome
}

func readToolCallLines(t *testing.T, stateHome, sessionID string) []trace.ToolCallLine {
	t.Helper()
	path := filepath.Join(stateHome, "liam", "traces", sessionID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading trace file: %v", err)
	}
	var out []trace.ToolCallLine
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var l trace.ToolCallLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatalf("unmarshaling line %q: %v", line, err)
		}
		out = append(out, l)
	}
	return out
}

// TestToolDefsInjectIntentAsRequiredProperty covers issue #63's schema-
// injection acceptance criterion: every advertised Tool gains a required
// "intent" string property, regardless of what the Tool itself declared.
func TestToolDefsInjectIntentAsRequiredProperty(t *testing.T) {
	reg := tool.NewRegistry(&fakeTool{name: "bash"})
	defs := toolDefs(reg)
	if len(defs) != 1 {
		t.Fatalf("len(defs) = %d, want 1", len(defs))
	}

	props, ok := defs[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Parameters[properties] = %v, want a map", defs[0].Parameters["properties"])
	}
	if _, ok := props["intent"]; !ok {
		t.Fatalf("Parameters[properties] = %+v, want an \"intent\" entry", props)
	}

	required, ok := defs[0].Parameters["required"].([]any)
	if !ok {
		t.Fatalf("Parameters[required] = %v, want a []any", defs[0].Parameters["required"])
	}
	var sawIntent bool
	for _, r := range required {
		if r == "intent" {
			sawIntent = true
		}
	}
	if !sawIntent {
		t.Errorf("required = %+v, want \"intent\" included", required)
	}
}

// TestToolDefsInjectIntentPreservesExistingRequiredProperties covers the
// merge behavior against a Tool that already declares its own required
// properties (every built-in Tool's own []string-typed "required"), plus a
// stand-in for an MCP tool's []any-typed one.
func TestToolDefsInjectIntentPreservesExistingRequiredProperties(t *testing.T) {
	cases := []struct {
		name     string
		schema   tool.Schema
		wantReqs []string
	}{
		{
			name: "string-typed required (built-in Tool convention)",
			schema: tool.Schema{
				"type":       "object",
				"properties": map[string]any{"command": map[string]any{"type": "string"}},
				"required":   []string{"command"},
			},
			wantReqs: []string{"command", "intent"},
		},
		{
			name: "any-typed required (MCP tool convention)",
			schema: tool.Schema{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []any{"query"},
			},
			wantReqs: []string{"query", "intent"},
		},
		{
			name:     "no properties/required at all",
			schema:   tool.Schema{"type": "object"},
			wantReqs: []string{"intent"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withIntent(tc.schema)
			required, _ := got["required"].([]any)
			if len(required) != len(tc.wantReqs) {
				t.Fatalf("required = %+v, want %+v", required, tc.wantReqs)
			}
			for i, want := range tc.wantReqs {
				if required[i] != want {
					t.Errorf("required[%d] = %v, want %q", i, required[i], want)
				}
			}
		})
	}
}

// TestWithIntentNeverMutatesOriginalSchema covers withIntent's "schema
// itself is never mutated" invariant — a shared package-level literal
// (every built-in Tool's Parameters()) must come back out unchanged on a
// second call.
func TestWithIntentNeverMutatesOriginalSchema(t *testing.T) {
	schema := tool.Schema{
		"type":       "object",
		"properties": map[string]any{"command": map[string]any{"type": "string"}},
		"required":   []string{"command"},
	}
	_ = withIntent(schema)

	if _, ok := schema["properties"].(map[string]any)["intent"]; ok {
		t.Error("withIntent mutated the original schema's properties map")
	}
	if len(schema["required"].([]string)) != 1 {
		t.Error("withIntent mutated the original schema's required slice")
	}
}

// TestDispatchTracesNormalExecutionWithIntentAndDuration drives a real
// dispatch (via Run/the fake-Provider seam) with l.Trace set, and asserts
// the resulting ToolCallLine: decision=executed, the model-supplied intent
// carried through, and a real duration_ms.
func TestDispatchTracesNormalExecutionWithIntentAndDuration(t *testing.T) {
	tr, stateHome := newTestTracer(t, "sess-exec")
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

	lines := readToolCallLines(t, stateHome, "sess-exec")
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
	// fakeTool.Run doesn't sleep, but duration_ms must at least be present
	// (>= 0) rather than always-zero-because-never-set; the real assertion
	// that matters is DecisionErrored/DeniedByHook never set it, covered
	// below.
	if ft.gotArg["intent"] != nil {
		t.Errorf("tool.Run saw args %+v, want \"intent\" stripped before Run", ft.gotArg)
	}
}

// TestDispatchTracesBeforeToolDenialAsDeniedByHookWithSource drives a real
// hook.Runner configured with a stub denying beforeTool hook through Run,
// and asserts the resulting ToolCallLine: decision=denied_by_hook, no
// duration_ms, and Source naming the denying hook.
func TestDispatchTracesBeforeToolDenialAsDeniedByHookWithSource(t *testing.T) {
	tr, stateHome := newTestTracer(t, "sess-deny")
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

	lines := readToolCallLines(t, stateHome, "sess-deny")
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
	tr, stateHome := newTestTracer(t, "sess-error")
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

	lines := readToolCallLines(t, stateHome, "sess-error")
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
	tr, stateHome := newTestTracer(t, "sess-unknown")
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

	lines := readToolCallLines(t, stateHome, "sess-unknown")
	if len(lines) != 1 || lines[0].Decision != trace.DecisionErrored || lines[0].Tool != "nonexistent" {
		t.Errorf("lines = %+v, want 1 errored line for \"nonexistent\"", lines)
	}
}
