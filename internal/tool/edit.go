package tool

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Edit replaces an exact, unique substring in a file with new content — a
// targeted find/replace rather than a full-file rewrite.
type Edit struct{}

func (Edit) Name() string { return "edit" }
func (Edit) Description() string {
	return "Replace an exact, unique substring in a file with new content."
}

func (Edit) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact text to replace; must appear exactly once in the file.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Text to replace it with.",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (Edit) Safety() Safety {
	return Safety{SideEffect: SideEffectWrite, Permission: PermissionAllow}
}

func (Edit) Run(_ context.Context, args map[string]any) Result {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return errorResult(`edit: "path" argument is required`)
	}
	oldString, ok := args["old_string"].(string)
	if !ok || oldString == "" {
		return errorResult(`edit: "old_string" argument is required`)
	}
	newString, _ := args["new_string"].(string)

	info, err := os.Stat(path)
	if err != nil {
		return errorResult(fmt.Sprintf("edit %s: %v", path, err))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errorResult(fmt.Sprintf("edit %s: %v", path, err))
	}
	content := string(data)

	switch count := strings.Count(content, oldString); count {
	case 0:
		return errorResult(fmt.Sprintf("edit %s: old_string not found", path))
	case 1:
		// exactly one match, proceed
	default:
		return errorResult(fmt.Sprintf("edit %s: old_string is not unique (%d matches)", path, count))
	}

	updated := strings.Replace(content, oldString, newString, 1)
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return errorResult(fmt.Sprintf("edit %s: %v", path, err))
	}
	return Result{Content: fmt.Sprintf("edited %s", path)}
}
