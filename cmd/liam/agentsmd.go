package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// gitRootTimeout bounds gitRoot's `git rev-parse` call, so a hung or
// misbehaving git binary can't block liam startup indefinitely.
const gitRootTimeout = 5 * time.Second

// loadAgentsMD resolves and concatenates the AGENTS.md content liam folds
// into its system prompt at startup: the global file
// ($XDG_CONFIG_HOME/liam/AGENTS.md, falling back to ~/.config/liam/AGENTS.md)
// first, then every AGENTS.md found walking from dir up to the git
// repository root containing dir (or the filesystem root, if dir isn't
// inside a git repository), collected in root-to-cwd order. Concatenation
// is raw text — no headers or labels distinguishing one file from another —
// matching pi's own discovery mechanism (see CONTEXT.md).
//
// A missing file at any level is skipped silently. A read error on a file
// that does exist (e.g. a permissions problem) prints a warning to errOut
// and skips just that file; it never fails the overall load.
func loadAgentsMD(dir string, errOut io.Writer) (string, error) {
	var parts []string

	globalPath, err := globalAgentsMDPath()
	if err != nil {
		return "", err
	}
	if content, ok := readAgentsMDFile(globalPath, errOut); ok {
		parts = append(parts, content)
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for _, path := range projectAgentsMDPaths(dir) {
		if content, ok := readAgentsMDFile(path, errOut); ok {
			parts = append(parts, content)
		}
	}

	return strings.Join(parts, "\n\n"), nil
}

// globalAgentsMDPath returns $XDG_CONFIG_HOME/liam/AGENTS.md, falling back
// to ~/.config/liam/AGENTS.md when XDG_CONFIG_HOME is unset.
func globalAgentsMDPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "liam", "AGENTS.md"), nil
}

// projectAgentsMDPaths returns the path an AGENTS.md would live at for
// every directory from dir (inclusive) up to the git repository root
// containing dir, or up to the filesystem root if dir isn't inside a git
// repository, in root-to-cwd order.
func projectAgentsMDPaths(dir string) []string {
	// Resolve symlinks so cur can be compared against git's (already
	// symlink-resolved) --show-toplevel output — otherwise e.g. macOS's
	// /tmp -> /private/tmp symlink would make the two never match, and
	// the walk would overshoot the git root.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	root := gitRoot(dir)

	var paths []string
	for cur := dir; ; {
		paths = append(paths, filepath.Join(cur, "AGENTS.md"))
		if cur == root {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	slices.Reverse(paths)
	return paths
}

// gitRoot returns the absolute path of the git repository root containing
// dir, or "" if dir isn't inside a git repository (or git isn't
// available, or it doesn't respond within gitRootTimeout) — the caller
// then walks all the way to the filesystem root instead.
func gitRoot(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitRootTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(string(out))
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root
}

// readAgentsMDFile reads path, returning ok=false with no warning if it
// simply doesn't exist, or ok=false with a warning printed to errOut for
// any other read error (e.g. a permissions problem).
func readAgentsMDFile(path string, errOut io.Writer) (content string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(errOut, "warning: reading %s: %v\n", path, err)
		}
		return "", false
	}
	return string(data), true
}
