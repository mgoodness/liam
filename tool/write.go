package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var writeDefinition = Definition{
	Name:        "write",
	Description: "Create or overwrite a file with the given content, auto-creating any missing parent directories.",
	Parameters: Parameters{
		Type: "object",
		Properties: map[string]Property{
			"path":    {Type: "string", Description: "Path to the file to create or overwrite."},
			"content": {Type: "string", Description: "The full content to write to the file."},
		},
		Required: []string{"path", "content"},
	},
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Write creates or overwrites the file at args.Path with args.Content,
// creating any missing parent directories first.
func Write(ctx context.Context, args json.RawMessage) (string, error) {
	var a writeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing write args: %w", err)
	}
	if a.Path == "" {
		return "", errors.New("write: path is required")
	}

	if dir := filepath.Dir(a.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(a.Path, []byte(a.Content), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", a.Path, err)
	}

	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path), nil
}
