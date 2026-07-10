package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var readDefinition = Definition{
	Name:        "read",
	Description: "Read a file's contents, optionally slicing by line offset/limit so large files can be read in slices.",
	Parameters: Parameters{
		Type: "object",
		Properties: map[string]Property{
			"path":   {Type: "string", Description: "Path to the file to read."},
			"offset": {Type: "integer", Description: "0-indexed line number to start reading from. Defaults to 0 (the start of the file)."},
			"limit":  {Type: "integer", Description: "Maximum number of lines to return. Defaults to the rest of the file."},
		},
		Required: []string{"path"},
	},
}

type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Read returns a file's contents, or a line-based [offset, offset+limit)
// slice of it when either is supplied.
func Read(ctx context.Context, args json.RawMessage) (string, error) {
	var a readArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing read args: %w", err)
	}
	if a.Path == "" {
		return "", errors.New("read: path is required")
	}

	data, err := os.ReadFile(a.Path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", a.Path, err)
	}

	if a.Offset == 0 && a.Limit == 0 {
		return string(data), nil
	}

	lines := strings.Split(string(data), "\n")

	start := a.Offset
	if start < 0 {
		start = 0
	}
	if start > len(lines) {
		start = len(lines)
	}

	end := len(lines)
	if a.Limit > 0 && start+a.Limit < end {
		end = start + a.Limit
	}

	return strings.Join(lines[start:end], "\n"), nil
}

// readSummarize renders the path being read, for progress reporting. It
// ignores a parse failure and returns the empty string, since Handler will
// itself fail on the same malformed args and report the error properly.
func readSummarize(args json.RawMessage) string {
	var a readArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ""
	}
	return a.Path
}
