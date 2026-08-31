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

func TestBashRunRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	got := (Bash{}).Run(ctx, map[string]any{"command": "sleep 5"})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true (command should have been killed)")
	}
}
