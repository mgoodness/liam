// Package xdg resolves XDG Base Directory paths shared by more than one
// package — currently just $XDG_STATE_HOME, needed by both
// internal/skill's trust store and internal/trace's per-session JSONL
// files. $XDG_CONFIG_HOME stays private to the packages that resolve it
// (internal/skill, internal/instructions) since nothing else currently
// duplicates that one.
package xdg

import (
	"os"
	"path/filepath"
)

// StateHome resolves $XDG_STATE_HOME, falling back to ~/.local/state per
// the XDG Base Directory spec.
func StateHome() (string, error) {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}
