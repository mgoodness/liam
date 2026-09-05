package hook

import (
	"context"
	"testing"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/trace"
	"github.com/mgoodness/liam/internal/trace/tracetest"
)

// TestRunTracesAfterToolHook covers issue #63's "every hook run produces its
// own trace line" criterion: an afterTool hook's run gets a HookRunLine
// recording its identity, exit code, and duration.
func TestRunTracesAfterToolHook(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-after")
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: "exit 0"}}},
		Trace: tr,
	}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	tr.Close()

	lines := tracetest.ReadLines[trace.HookRunLine](t, stateHome, "sess-after")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
	if lines[0].Lifecycle != "afterTool" || lines[0].Command != "exit 0" || lines[0].ExitCode != 0 {
		t.Errorf("line = %+v, want {Lifecycle: afterTool, Command: \"exit 0\", ExitCode: 0}", lines[0])
	}
}

// TestRunTracesFailingHook covers a hook that runs and returns non-zero: it
// still gets its own trace line, with the failure's exit code and stderr,
// even though nothing gates on it.
func TestRunTracesFailingHook(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-fail")
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: `echo "no" >&2; exit 1`}}},
		Trace: tr,
	}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	tr.Close()

	lines := tracetest.ReadLines[trace.HookRunLine](t, stateHome, "sess-fail")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
	if lines[0].ExitCode != 1 || lines[0].Stderr != "no" {
		t.Errorf("line = %+v, want {ExitCode: 1, Stderr: \"no\"}", lines[0])
	}
}

// TestRunTracesFailOpenHook covers a fail-open condition (ADR-0002): the
// hook run still gets its own trace line, even though it never actually ran
// the configured command.
func TestRunTracesFailOpenHook(t *testing.T) {
	tr, stateHome := tracetest.New(t, "sess-failopen")
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: "/no/such/binary-liam-test"}}},
		Trace: tr,
	}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	tr.Close()

	lines := tracetest.ReadLines[trace.HookRunLine](t, stateHome, "sess-failopen")
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1: %+v", len(lines), lines)
	}
}
