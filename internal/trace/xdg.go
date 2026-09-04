package trace

import (
	"os"
	"path/filepath"
)

// xdgStateHome resolves $XDG_STATE_HOME, falling back to ~/.local/state per
// the XDG Base Directory spec. Duplicated from internal/skill's own
// xdgStateHome rather than shared — there's no common "xdg" package in this
// codebase yet (see internal/skill/xdg.go), and this one helper isn't worth
// introducing one for.
func xdgStateHome() (string, error) {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

// tracesDir resolves $XDG_STATE_HOME/liam/traces, the ticket's spec'd
// directory for every session's JSONL trace file.
func tracesDir() (string, error) {
	base, err := xdgStateHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "liam", "traces"), nil
}
