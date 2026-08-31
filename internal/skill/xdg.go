package skill

import (
	"os"
	"path/filepath"
)

// xdgConfigHome resolves $XDG_CONFIG_HOME, falling back to ~/.config per
// the XDG Base Directory spec.
func xdgConfigHome() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// xdgStateHome resolves $XDG_STATE_HOME, falling back to ~/.local/state
// per the XDG Base Directory spec.
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
