package main

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
	"github.com/mgoodness/liam/internal/trace"
)

// TestRunHeadlessWiresTraceToRealSessionFile covers issue #63's end-to-end
// wiring through the real binary entry point: runHeadless must attach a
// live *trace.Writer to both loop.Trace and loop.Hooks.Trace, point it at
// the same session ID loop.Hooks itself uses, and Close it (draining
// pending writes) before returning — leaving a readable JSONL file behind
// recording the turn's tool call and the hook run that gated it.
func TestRunHeadlessWiresTraceToRealSessionFile(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	tracer := trace.New()
	hooks := &hook.Runner{
		Hooks: config.HooksConfig{BeforeTool: []config.HookConfig{{Command: "exit 0"}}},
		Trace: tracer,
	}
	ft := &fakeTracedTool{result: tool.Result{Content: "done"}}
	loop := agent.Loop{
		Provider: &toolCallThenDoneProvider{},
		Tools:    tool.NewRegistry(ft),
		Hooks:    hooks,
		Trace:    tracer,
	}

	var stdout, stderr bytes.Buffer
	code := runHeadless(loop, nil, config.Config{}, "hi", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runHeadless() = %d, want 0; stderr = %q", code, stderr.String())
	}
	if hooks.SessionID == "" {
		t.Fatal("hooks.SessionID was never set")
	}

	path := filepath.Join(stateHome, "liam", "traces", hooks.SessionID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading trace file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	var sawToolCall, sawHookRun bool
	for _, line := range lines {
		var probe struct {
			Tool      string `json:"tool"`
			Lifecycle string `json:"lifecycle"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("unmarshaling line %q: %v", line, err)
		}
		if probe.Tool == "fake" {
			sawToolCall = true
		}
		if probe.Lifecycle == "beforeTool" {
			sawHookRun = true
		}
	}
	if !sawToolCall {
		t.Errorf("lines = %v, want a ToolCallLine for the \"fake\" tool", lines)
	}
	if !sawHookRun {
		t.Errorf("lines = %v, want a HookRunLine for the beforeTool hook", lines)
	}
}

// fakeTracedTool is a minimal tool.Tool for TestRunHeadlessWiresTraceToRealSessionFile.
type fakeTracedTool struct {
	result tool.Result
}

func (f *fakeTracedTool) Name() string            { return "fake" }
func (f *fakeTracedTool) Description() string     { return "fake tool" }
func (f *fakeTracedTool) Parameters() tool.Schema { return tool.Schema{"type": "object"} }
func (f *fakeTracedTool) Safety() tool.Safety {
	return tool.Safety{SideEffect: tool.SideEffectRead}
}
func (f *fakeTracedTool) Run(context.Context, map[string]any) tool.Result { return f.result }

// toolCallThenDoneProvider scripts a single ToolCallEvent against the
// "fake" tool on its first Stream call, then a plain DoneEvent on every
// subsequent call — enough for agent.Loop.Run to dispatch exactly one tool
// call before ending the turn. called must live on the type, not a local
// inside Stream, so it persists across the multiple Stream calls one Run
// makes across turns.
type toolCallThenDoneProvider struct {
	called bool
}

func (toolCallThenDoneProvider) Name() string { return "tool-call-then-done" }
func (p *toolCallThenDoneProvider) Stream(context.Context, provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		if !p.called {
			p.called = true
			if !yield(provider.ToolCallEvent{ID: "call_1", Name: "fake", ArgsJSON: `{}`}, nil) {
				return
			}
			yield(provider.DoneEvent{FinishReason: "tool_calls"}, nil)
			return
		}
		yield(provider.DoneEvent{FinishReason: "stop"}, nil)
	}
}
