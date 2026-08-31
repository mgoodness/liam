package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Bash runs a shell command via "sh -c" and returns its combined stdout and
// stderr, truncated at outputCap bytes (ADR-0005).
type Bash struct{}

func (Bash) Name() string { return "bash" }
func (Bash) Description() string {
	return "Run a shell command and return its combined stdout and stderr. Output over ~8000 bytes is truncated."
}

func (Bash) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to run.",
			},
		},
		"required": []string{"command"},
	}
}

func (Bash) Safety() Safety {
	return Safety{SideEffect: SideEffectShell}
}

func (Bash) Run(ctx context.Context, args map[string]any) Result {
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return errorResult(`bash: "command" argument is required`)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		// truncate() only the captured output, then always append the
		// error diagnostic afterward — otherwise a command that both fails
		// and produces huge output could have its "error: ..." line cut
		// away entirely, leaving IsError true with no visible reason.
		return Result{Content: fmt.Sprintf("%s\nerror: %v", truncate(out.String()), err), IsError: true}
	}
	return Result{Content: truncate(out.String())}
}
