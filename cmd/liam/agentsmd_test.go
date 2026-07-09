package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFile writes content to path, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// initGitRepo runs `git init` in dir so it becomes a git repository root.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}

func TestLoadAgentsMD_GlobalOnly(t *testing.T) {
	xdg := t.TempDir()
	writeFile(t, filepath.Join(xdg, "liam", "AGENTS.md"), "global instructions")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	dir := t.TempDir() // no AGENTS.md, no git repo

	var errOut bytes.Buffer
	got, err := loadAgentsMD(dir, &errOut)
	if err != nil {
		t.Fatalf("loadAgentsMD: %v", err)
	}
	if got != "global instructions" {
		t.Errorf("got %q, want %q", got, "global instructions")
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr output: %q", errOut.String())
	}
}

func TestLoadAgentsMD_MissingFilesSkippedSilently(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no global AGENTS.md written

	dir := t.TempDir()

	var errOut bytes.Buffer
	got, err := loadAgentsMD(dir, &errOut)
	if err != nil {
		t.Fatalf("loadAgentsMD: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
	if errOut.Len() != 0 {
		t.Errorf("missing files should be silent, got stderr: %q", errOut.String())
	}
}

func TestLoadAgentsMD_ConcatenatesGlobalThenProjectRootToCwdOrder(t *testing.T) {
	xdg := t.TempDir()
	writeFile(t, filepath.Join(xdg, "liam", "AGENTS.md"), "global")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	repo := t.TempDir()
	initGitRepo(t, repo)
	writeFile(t, filepath.Join(repo, "AGENTS.md"), "repo-root")

	mid := filepath.Join(repo, "pkg")
	// no AGENTS.md at this level: should be skipped silently.

	leaf := filepath.Join(mid, "sub")
	writeFile(t, filepath.Join(leaf, "AGENTS.md"), "leaf")

	var errOut bytes.Buffer
	got, err := loadAgentsMD(leaf, &errOut)
	if err != nil {
		t.Fatalf("loadAgentsMD: %v", err)
	}

	wantOrder := []string{"global", "repo-root", "leaf"}
	idx := -1
	for _, want := range wantOrder {
		i := strings.Index(got, want)
		if i == -1 {
			t.Fatalf("expected %q to appear in result, got %q", want, got)
		}
		if i <= idx {
			t.Errorf("expected %q to appear after previous parts (root-to-cwd order), got %q", want, got)
		}
		idx = i
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr output: %q", errOut.String())
	}
}

func TestLoadAgentsMD_NonGitDirectoryWalksToFilesystemRootWithoutErroring(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	top := t.TempDir() // not a git repo
	leaf := filepath.Join(top, "a", "b", "c")
	writeFile(t, filepath.Join(leaf, "AGENTS.md"), "leaf-in-non-git-tree")
	writeFile(t, filepath.Join(top, "a", "AGENTS.md"), "mid-in-non-git-tree")

	var errOut bytes.Buffer
	got, err := loadAgentsMD(leaf, &errOut)
	if err != nil {
		t.Fatalf("loadAgentsMD outside a git repo should not error, got: %v", err)
	}

	iMid := strings.Index(got, "mid-in-non-git-tree")
	iLeaf := strings.Index(got, "leaf-in-non-git-tree")
	if iMid == -1 || iLeaf == -1 {
		t.Fatalf("expected both non-git-tree fixtures in result, got %q", got)
	}
	if iMid >= iLeaf {
		t.Errorf("expected mid-level file before leaf (root-to-cwd order), got %q", got)
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr output: %q", errOut.String())
	}
}

func TestLoadAgentsMD_ReadErrorWarnsAndSkipsJustThatFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits aren't enforced the same way on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses unix permission bits")
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	repo := t.TempDir()
	initGitRepo(t, repo)
	badPath := filepath.Join(repo, "AGENTS.md")
	writeFile(t, badPath, "unreadable")
	if err := os.Chmod(badPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(badPath, 0o644) })

	leaf := filepath.Join(repo, "sub")
	writeFile(t, filepath.Join(leaf, "AGENTS.md"), "readable-leaf")

	var errOut bytes.Buffer
	got, err := loadAgentsMD(leaf, &errOut)
	if err != nil {
		t.Fatalf("a read error on one file should not fail the whole load, got: %v", err)
	}
	if !strings.Contains(got, "readable-leaf") {
		t.Errorf("expected the readable leaf file's content to still be included, got %q", got)
	}
	if strings.Contains(got, "unreadable") {
		t.Errorf("unreadable file's content should have been skipped, got %q", got)
	}
	if errOut.Len() == 0 {
		t.Error("expected a warning on stderr for the unreadable file")
	}
	if !strings.Contains(errOut.String(), badPath) {
		t.Errorf("expected warning to name the unreadable path %s, got %q", badPath, errOut.String())
	}
}

func TestGlobalAgentsMDPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg-config")

	got, err := globalAgentsMDPath()
	if err != nil {
		t.Fatalf("globalAgentsMDPath: %v", err)
	}

	want := filepath.Join("/xdg-config", "liam", "AGENTS.md")
	if got != want {
		t.Errorf("globalAgentsMDPath = %q, want %q", got, want)
	}
}

func TestGlobalAgentsMDPathFallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/testuser")

	got, err := globalAgentsMDPath()
	if err != nil {
		t.Fatalf("globalAgentsMDPath: %v", err)
	}

	want := filepath.Join("/home/testuser", ".config", "liam", "AGENTS.md")
	if got != want {
		t.Errorf("globalAgentsMDPath = %q, want %q", got, want)
	}
}
