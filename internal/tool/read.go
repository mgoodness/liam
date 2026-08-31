package tool

import (
	"context"
	"fmt"
	"os"
)

// Read reads a file's full contents.
type Read struct{}

func (Read) Name() string        { return "read" }
func (Read) Description() string { return "Read a file's contents." }

func (Read) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read.",
			},
		},
		"required": []string{"path"},
	}
}

func (Read) Safety() Safety {
	return Safety{SideEffect: SideEffectRead}
}

func (Read) Run(_ context.Context, args map[string]any) Result {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return errorResult(`read: "path" argument is required`)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return errorResult(fmt.Sprintf("read %s: %v", path, err))
	}
	return Result{Content: string(data)}
}
