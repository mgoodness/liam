package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GlobalDirs returns liam's global skills directories, in ascending
// precedence order — a name found in a later directory wins (see
// Discover) — so callers pass this slice through unchanged to get that
// precedence for free:
//
//  1. $XDG_CONFIG_HOME/agents/skills, falling back to
//     ~/.config/agents/skills when XDG_CONFIG_HOME is unset — the same XDG
//     resolution provider.credentialsPath uses.
//  2. ~/.agents/skills, checked regardless of XDG_CONFIG_HOME. This is the
//     vendor-neutral location the wider agent-skills ecosystem actually
//     uses today, which doesn't respect XDG_CONFIG_HOME — so it's checked
//     unconditionally rather than folded into the XDG path above, and
//     wins on a name collision between the two since a skill placed there
//     is the more deliberate, actively-maintained one.
//
// Neither path is namespaced to liam itself, unlike the discovery paths
// this replaced — see ADR 0008. Both are vendor-neutral so a skill
// authored once is discoverable by liam and any other Agent-Skills-
// compatible tool without copying or symlinking it per tool.
func GlobalDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}

	return []string{
		filepath.Join(xdg, "agents", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}, nil
}

// ProjectDir resolves liam's project skills directory: .agents/skills
// under the git repository root containing dir, or under dir itself if
// dir isn't inside a git repository. This is resolved once, not walked
// and collected at every directory level the way AGENTS.md discovery is —
// skills are discrete named units, not layerable prose. Vendor-neutral
// like GlobalDirs, replacing what was .liam/skills — see ADR 0008.
func ProjectDir(dir string) string {
	root, ok := gitRoot(dir)
	if !ok {
		root = dir
	}
	return filepath.Join(root, ".agents", "skills")
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
