// Package instructions discovers and assembles project-instruction files —
// AGENTS.md and LIAM.md — into the text used as a provider.Request's
// SystemPrompt.
package instructions

import (
	"os"
	"path/filepath"
	"strings"
)

// maxFileBytes caps how much of any single instruction file is read;
// maxTotalBytes caps the assembled result across every file combined.
const (
	maxFileBytes  = 8 * 1024
	maxTotalBytes = 32 * 1024
)

// candidateNames are checked, in order, in every directory along the walk —
// AGENTS.md (the cross-client convention) before LIAM.md (liam's own).
var candidateNames = []string{"AGENTS.md", "LIAM.md"}

// Load assembles the system prompt from project-instruction files: the
// personal $XDG_CONFIG_HOME/liam/LIAM.md loads first (most general),
// followed by every AGENTS.md/LIAM.md found walking from the git root (or
// cwd, if cwd isn't inside a git repo) down to cwd, concatenated
// general-to-specific. Each file is capped at maxFileBytes; the assembled
// result is capped at maxTotalBytes, truncating whichever file crosses the
// boundary. A missing file at any location is not an error — "" is
// returned when nothing is found. The only error Load itself returns is a
// failure to resolve the user's home directory while locating the personal
// config file.
func Load(cwd string) (string, error) {
	var paths []string

	personal, err := personalPath()
	if err != nil {
		return "", err
	}
	if personal != "" {
		paths = append(paths, personal)
	}

	root := gitRoot(cwd)
	for _, dir := range dirChain(root, cwd) {
		for _, name := range candidateNames {
			p := filepath.Join(dir, name)
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				paths = append(paths, p)
			}
		}
	}

	return assemble(paths), nil
}

// assemble concatenates the file at each of paths, in order, separated by a
// blank line, applying the per-file and total size caps.
func assemble(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		remaining := maxTotalBytes - b.Len()
		if remaining <= 0 {
			break
		}

		data, err := os.ReadFile(p)
		if err != nil {
			continue // race with the Stat above, or a permission error: skip
		}
		if len(data) > maxFileBytes {
			data = data[:maxFileBytes]
		}

		if b.Len() > 0 {
			const sep = "\n\n"
			if remaining < len(sep) {
				break
			}
			b.WriteString(sep)
			remaining -= len(sep)
		}

		if len(data) > remaining {
			data = data[:remaining]
		}
		b.Write(data)
	}
	return b.String()
}

// personalPath returns $XDG_CONFIG_HOME/liam/LIAM.md (falling back to
// ~/.config when XDG_CONFIG_HOME is unset, per the XDG base directory
// spec), or "" if no such file exists. A non-nil error means resolving the
// home directory itself failed, not that the file is merely absent.
func personalPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	path := filepath.Join(base, "liam", "LIAM.md")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", nil
	}
	return path, nil
}

// gitRoot walks up from cwd looking for a .git entry (a directory for an
// ordinary repo, or a file for a worktree/submodule), returning the first
// directory found — or cwd itself if none is found up to the filesystem
// root.
func gitRoot(cwd string) string {
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

// dirChain returns every directory from root down to cwd, root first —
// []string{root} if cwd isn't under root.
func dirChain(root, cwd string) []string {
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return []string{root}
	}
	parts := strings.Split(rel, string(filepath.Separator))
	chain := make([]string, 0, len(parts)+1)
	d := root
	chain = append(chain, d)
	for _, p := range parts {
		d = filepath.Join(d, p)
		chain = append(chain, d)
	}
	return chain
}
