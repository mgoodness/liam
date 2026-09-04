package hook

import (
	"context"
	"testing"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/trace"
	"github.com/mgoodness/liam/internal/trace/tracetest"
)

// TestRunTracesAllowingHook covers issue #63's "every hook run produces its
// own trace line" criterion for the ordinary case: a beforeTool hook that
// allows the call still gets a HookRunLine recording its identity, exit
// code, and duration.
func TestRunTracesAllowingHook(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-allow")
	r := &Runner{
		Hooks: config.HooksConfig{BeforeTool: []config.HookConfig{{Command: "exit 0"}}},
		Trace: tr,
	}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Fatalf("Decision = %+v, want not blocked", d)
	}
	tr.Close()

	lines := tracetest.ReadLines[trace.HookRunLine](t, stateHome, "sess-allow")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
	if lines[0].Lifecycle != "beforeTool" || lines[0].Command != "exit 0" || lines[0].ExitCode != 0 {
		t.Errorf("line = %+v, want {Lifecycle: beforeTool, Command: \"exit 0\", ExitCode: 0}", lines[0])
	}
}

// TestRunTracesDenyingHookProducesHookRunAndCarriesSource covers issue #63's
// "a beforeTool denial produces both a hook-run line and a denied_by_hook
// tool-call line" criterion — this test covers the hook-run half (the
// tool-call half is internal/agent's responsibility) and checks Decision.Source
// carries the denying hook's identity for that tool-call line.
func TestRunTracesDenyingHookProducesHookRunAndCarriesSource(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-deny")
	r := &Runner{
		Hooks: config.HooksConfig{BeforeTool: []config.HookConfig{{Command: `echo "no" >&2; exit 1`}}},
		Trace: tr,
	}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if !d.Blocked {
		t.Fatalf("Decision = %+v, want blocked", d)
	}
	if d.Source != `echo "no" >&2; exit 1` {
		t.Errorf("Decision.Source = %q, want the denying hook's command", d.Source)
	}
	tr.Close()

	lines := tracetest.ReadLines[trace.HookRunLine](t, stateHome, "sess-deny")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
	if lines[0].ExitCode != 1 || lines[0].Stderr != "no" {
		t.Errorf("line = %+v, want {ExitCode: 1, Stderr: \"no\"}", lines[0])
	}
}

// TestRunTracesFailOpenHook covers a fail-open condition (ADR-0002): the
// hook run still gets its own trace line, even though it never produced a
// real policy verdict.
func TestRunTracesFailOpenHook(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-failopen")
	r := &Runner{
		Hooks: config.HooksConfig{BeforeTool: []config.HookConfig{{Command: "/no/such/binary-liam-test"}}},
		Trace: tr,
	}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Fatalf("Decision = %+v, want not blocked (fail open)", d)
	}
	tr.Close()

	lines := tracetest.ReadLines[trace.HookRunLine](t, stateHome, "sess-failopen")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
}

// TestRunTracesAfterToolHook covers "every hook run produces its own trace
// line": afterTool's own lifecycle point, unrelated to BeforeTool's gating,
// still gets traced.
func TestRunTracesAfterToolHook(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-after")
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: "exit 0"}}},
		Trace: tr,
	}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	tr.Close()

	lines := tracetest.ReadLines[trace.HookRunLine](t, stateHome, "sess-after")
	if len(lines) != 1 || lines[0].Lifecycle != "afterTool" {
		t.Errorf("lines = %+v, want 1 afterTool HookRunLine", lines)
	}
}
