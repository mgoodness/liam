package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// globalConfigPath returns the global config file location,
// $XDG_CONFIG_HOME/liam/liam.jsonc (falling back to ~/.config when
// XDG_CONFIG_HOME is unset, per the XDG base directory spec). path is ""
// when no file exists there; a non-nil error means the existence check
// itself failed for a reason other than the file being absent (e.g.
// permission denied), which callers should surface rather than silently
// treat as "no config".
func globalConfigPath() (path string, err error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	path = filepath.Join(base, "liam", "liam.jsonc")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return path, nil
}

// findProjectConfig walks up from startDir looking for a liam.jsonc file,
// returning the first one found. path is "" when none exists all the way up
// to the filesystem root; a non-nil error means a stat call failed for a
// reason other than the file being absent.
func findProjectConfig(startDir string) (path string, err error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "liam.jsonc")
		_, statErr := os.Stat(candidate)
		switch {
		case statErr == nil:
			return candidate, nil
		case !errors.Is(statErr, fs.ErrNotExist):
			return "", statErr
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
