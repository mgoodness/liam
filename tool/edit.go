package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var editDefinition = Definition{
	Name:        "edit",
	Description: "Replace an exact, uniquely-matching substring in a file with new text.",
	Parameters: Parameters{
		Type: "object",
		Properties: map[string]Property{
			"path":     {Type: "string", Description: "Path to the file to edit."},
			"old_text": {Type: "string", Description: "The exact text to replace. Must match exactly once in the file."},
			"new_text": {Type: "string", Description: "The text to replace old_text with."},
		},
		Required: []string{"path", "old_text", "new_text"},
	},
}

type editArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

// Edit replaces args.OldText with args.NewText in the file at args.Path.
// It never guesses which occurrence to replace: zero matches or more than
// one match returns an error and leaves the file untouched.
func Edit(ctx context.Context, args json.RawMessage) (string, error) {
	var a editArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing edit args: %w", err)
	}
	if a.Path == "" {
		return "", errors.New("edit: path is required")
	}
	if a.OldText == "" {
		return "", errors.New("edit: old_text is required")
	}

	data, err := os.ReadFile(a.Path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", a.Path, err)
	}
	content := string(data)

	switch n := strings.Count(content, a.OldText); n {
	case 0:
		return "", fmt.Errorf("edit: old_text not found in %s", a.Path)
	case 1:
		// exactly one match — proceed
	default:
		return "", fmt.Errorf("edit: old_text matches %d times in %s, must be unique", n, a.Path)
	}

	updated := strings.Replace(content, a.OldText, a.NewText, 1)
	if err := os.WriteFile(a.Path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", a.Path, err)
	}

	return fmt.Sprintf("edited %s", a.Path), nil
}
