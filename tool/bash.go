package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// DefaultTimeout is how long a bash command may run before it's killed,
// unless a call supplies its own Timeout. A var (not a const) so it stays
// overridable for testing.
var DefaultTimeout = 30 * time.Second

var bashDefinition = Definition{
	Name:        "bash",
	Description: "Execute a shell command synchronously, capturing its stdout, stderr, and exit code.",
	Parameters: Parameters{
		Type: "object",
		Properties: map[string]Property{
			"command": {Type: "string", Description: "The shell command to execute."},
			"timeout": {Type: "integer", Description: "Optional per-call timeout in seconds, overriding the default."},
		},
		Required: []string{"command"},
	},
}

type bashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// Bash runs args.Command via /bin/sh -c, killing it if it exceeds its
// timeout (args.Timeout seconds if set, otherwise DefaultTimeout).
func Bash(ctx context.Context, args json.RawMessage) (string, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing bash args: %w", err)
	}
	if a.Command == "" {
		return "", errors.New("bash: command is required")
	}

	timeout := DefaultTimeout
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", a.Command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("bash: command timed out after %s", timeout)
	}

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return "", fmt.Errorf("bash: %w", runErr)
		}
		exitCode = exitErr.ExitCode()
	}

	return fmt.Sprintf("exit code: %d\nstdout:\n%s\nstderr:\n%s", exitCode, stdout.String(), stderr.String()), nil
}
