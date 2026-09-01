// Package shellrun runs a shell command via "sh -c", feeding it a stdin
// payload and extra environment variables and capturing its stdout/
// stderr — the exec-plumbing shape shared by internal/hook (which gates
// tool calls on the exit code, per ADR-0002's fail-open rules) and
// internal/statusline (which treats any failure uniformly). Each caller
// applies its own interpretation to the same raw Result; this package
// makes no policy judgement about what a given exit code or error means.
package shellrun

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// Result is one "sh -c" invocation's raw outcome.
type Result struct {
	// ExitCode is the process's exit code once it ran to completion,
	// including a non-zero or signal-killed (-1, per os/exec's documented
	// behavior) exit — meaningful only when Err is nil.
	ExitCode int
	Stdout   string
	Stderr   string
	// Err is set only when the command couldn't be run to completion at
	// all — sh itself failed to start, or ctx was canceled/timed out
	// before it exited. A non-zero ExitCode is never reported here.
	Err error
}

// Run executes command via "sh -c", with dir as its working directory
// (the caller's own, when dir == ""), stdin written to the process's
// standard input, and extraEnv appended to its environment.
func Run(ctx context.Context, command string, stdin []byte, dir string, extraEnv []string) Result {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return Result{Stdout: stdout.String(), Stderr: stderr.String()}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{ExitCode: exitErr.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
	}
	return Result{Err: err, Stderr: stderr.String()}
}
