package tool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBashSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectShell}
	if got := (Bash{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestBashRunCapturesStdout(t *testing.T) {
	got := (Bash{}).Run(context.Background(), map[string]any{"command": "echo hello"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if strings.TrimSpace(got.Content) != "hello" {
		t.Errorf("Content = %q, want %q", got.Content, "hello")
	}
}

func TestBashRunCapturesStderr(t *testing.T) {
	got := (Bash{}).Run(context.Background(), map[string]any{"command": "echo oops 1>&2"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if !strings.Contains(got.Content, "oops") {
		t.Errorf("Content = %q, want it to contain %q", got.Content, "oops")
	}
}

func TestBashRunNonZeroExit(t *testing.T) {
	got := (Bash{}).Run(context.Background(), map[string]any{"command": "exit 7"})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestBashRunMissingCommandArg(t *testing.T) {
	got := (Bash{}).Run(context.Background(), map[string]any{})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

// TestBashRunTruncatesLargeOutput covers ADR-0005: bash's combined
// stdout+stderr is capped at outputCap chars, matching web_fetch's own
// convention.
func TestBashRunTruncatesLargeOutput(t *testing.T) {
	// yes | head prints far more than outputCap chars of newline-separated
	// output, so the result must come back truncated with a marker.
	got := (Bash{}).Run(context.Background(), map[string]any{"command": "yes | head -c 20000"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if len(got.Content) >= 20000 {
		t.Errorf("Content len = %d, want truncated well under the raw 20000-byte output", len(got.Content))
	}
	if !strings.Contains(got.Content, "truncated") {
		t.Errorf("Content = %q, want a truncation marker", got.Content)
	}
}

// TestBashRunAtCapReturnsUnchanged covers the exactly-at-cap boundary at
// the tool level (not just truncate() in isolation, per issue #85's
// acceptance criteria): output of exactly outputCap bytes must come back
// with no marker and no bytes dropped.
func TestBashRunAtCapReturnsUnchanged(t *testing.T) {
	got := (Bash{}).Run(context.Background(), map[string]any{"command": "head -c 8000 /dev/zero | tr '\\0' 'x'"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	want := strings.Repeat("x", outputCap)
	if got.Content != want {
		t.Errorf("Content len = %d, want exactly outputCap (%d) bytes unchanged", len(got.Content), outputCap)
	}
}

// TestBashRunPreservesErrorDiagnosticWhenOutputIsTruncated covers a bug
// truncate() alone would introduce: a command that produces huge output
// AND fails must not have its "error: ..." diagnostic silently swallowed
// by truncation — the diagnostic is what tells the model (and the user)
// the call failed at all.
func TestBashRunPreservesErrorDiagnosticWhenOutputIsTruncated(t *testing.T) {
	got := (Bash{}).Run(context.Background(), map[string]any{"command": "yes | head -c 20000; exit 7"})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
	if !strings.Contains(got.Content, "error:") {
		t.Errorf("Content lost its error diagnostic to truncation: %q", got.Content[max(0, len(got.Content)-200):])
	}
}

func TestBashRunRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	got := (Bash{}).Run(ctx, map[string]any{"command": "sleep 5"})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true (command should have been killed)")
	}
}
