package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GlobalDir returns $XDG_CONFIG_HOME/liam/skills, falling back to
// ~/.config/liam/skills when XDG_CONFIG_HOME is unset — the same XDG
// resolution provider.credentialsPath uses.
func GlobalDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "liam", "skills"), nil
}

// ProjectDir resolves liam's project skills directory: .liam/skills under
// the git repository root containing dir, or under dir itself if dir
// isn't inside a git repository. This is resolved once, not walked and
// collected at every directory level the way AGENTS.md discovery is —
// skills are discrete named units, not layerable prose.
func ProjectDir(dir string) string {
	root, ok := gitRoot(dir)
	if !ok {
		root = dir
	}
	return filepath.Join(root, ".liam", "skills")
}

// gitRoot returns the git repository root containing dir, and whether dir
// is inside a git repository at all.
func gitRoot(dir string) (string, bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
