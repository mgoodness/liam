package shellrun

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunCapturesStdoutAndStderr(t *testing.T) {
	res := Run(context.Background(), `echo out; echo err >&2`, nil, "", nil)
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "out" {
		t.Errorf("Stdout = %q, want %q", res.Stdout, "out")
	}
	if strings.TrimSpace(res.Stderr) != "err" {
		t.Errorf("Stderr = %q, want %q", res.Stderr, "err")
	}
}

func TestRunReportsNonZeroExitCodeWithoutErr(t *testing.T) {
	res := Run(context.Background(), "exit 3", nil, "", nil)
	if res.Err != nil {
		t.Errorf("Err = %v, want nil (a non-zero exit is a normal Result, not an Err)", res.Err)
	}
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res.ExitCode)
	}
}

func TestRunReportsSignalKillAsExitCodeNegativeOne(t *testing.T) {
	res := Run(context.Background(), "kill -9 $$", nil, "", nil)
	if res.Err != nil {
		t.Errorf("Err = %v, want nil", res.Err)
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 (os/exec's documented signal-kill convention)", res.ExitCode)
	}
}

func TestRunFeedsStdin(t *testing.T) {
	res := Run(context.Background(), "cat", []byte("hello"), "", nil)
	if res.Stdout != "hello" {
		t.Errorf("Stdout = %q, want %q", res.Stdout, "hello")
	}
}

func TestRunSetsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(), "pwd", nil, dir, nil)
	if got := strings.TrimSpace(res.Stdout); got != dir {
		t.Errorf("pwd = %q, want %q", got, dir)
	}
}

func TestRunAppendsExtraEnv(t *testing.T) {
	res := Run(context.Background(), `echo "$FOO"`, nil, "", []string{"FOO=bar"})
	if got := strings.TrimSpace(res.Stdout); got != "bar" {
		t.Errorf("$FOO = %q, want %q", got, "bar")
	}
}

func TestRunReportsErrOnCommandNotFound(t *testing.T) {
	// sh's own 127 "command not found" convention still surfaces as a
	// normal ExitCode, not Err — Err is reserved for sh itself failing to
	// start, which a missing $PATH can trigger.
	oldPath := ""
	t.Setenv("PATH", oldPath)
	res := Run(context.Background(), "true", nil, "", nil)
	if res.Err == nil {
		t.Fatal("Err = nil, want an error when sh itself can't be found on $PATH")
	}
}

func TestRunRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Run(ctx, "sleep 1", nil, "", nil)
	if res.Err == nil {
		t.Error("Err = nil, want an error for an already-canceled context")
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Errorf("Err = %v, want it to wrap context.Canceled", res.Err)
	}
}
