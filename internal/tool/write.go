package tool

import (
	"context"
	"fmt"
	"os"
)

// Write writes content to a file, creating or overwriting it.
type Write struct{}

func (Write) Name() string { return "write" }
func (Write) Description() string {
	return "Write content to a file, creating it or overwriting it if it already exists."
}

func (Write) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (Write) Safety() Safety {
	return Safety{SideEffect: SideEffectWrite}
}

func (Write) Run(_ context.Context, args map[string]any) Result {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return errorResult(`write: "path" argument is required`)
	}
	content, ok := args["content"].(string)
	if !ok {
		return errorResult(`write: "content" argument is required`)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return errorResult(fmt.Sprintf("write %s: %v", path, err))
	}
	return Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}
}
