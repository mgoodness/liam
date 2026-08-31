package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateHome points HOME and XDG_CONFIG_HOME at fresh temp dirs, so Load's
// personal-config lookup never touches the real filesystem outside the
// test fixture. Mirrors internal/skill's helper of the same name.
func isolateHome(t *testing.T) (home, xdgConfigHome string) {
	t.Helper()
	home = t.TempDir()
	xdgConfigHome = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	return home, xdgConfigHome
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadReturnsEmptyWhenNothingFound(t *testing.T) {
	isolateHome(t)
	cwd := t.TempDir()

	got, err := Load(cwd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "" {
		t.Errorf("Load() = %q, want \"\"", got)
	}
}

func TestLoadOrdersPersonalBeforeProjectGeneralToSpecific(t *testing.T) {
	_, xdgConfigHome := isolateHome(t)
	writeFile(t, filepath.Join(xdgConfigHome, "liam", "LIAM.md"), "personal")

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("Mkdir .git: %v", err)
	}
	writeFile(t, filepath.Join(root, "AGENTS.md"), "root")
	sub := filepath.Join(root, "a", "b")
	writeFile(t, filepath.Join(sub, "AGENTS.md"), "leaf")

	got, err := Load(sub)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := "personal\n\nroot\n\nleaf"
	if got != want {
		t.Errorf("Load() = %q, want %q", got, want)
	}
}

func TestLoadOrdersAGENTSBeforeLIAMWithinSameDirectory(t *testing.T) {
	isolateHome(t)
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "LIAM.md"), "liam-file")
	writeFile(t, filepath.Join(cwd, "AGENTS.md"), "agents-file")

	got, err := Load(cwd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := "agents-file\n\nliam-file"
	if got != want {
		t.Errorf("Load() = %q, want %q", got, want)
	}
}

func TestLoadFallsBackToCwdWithoutGit(t *testing.T) {
	isolateHome(t)
	parent := t.TempDir()
	writeFile(t, filepath.Join(parent, "AGENTS.md"), "should not be picked up")
	cwd := filepath.Join(parent, "sub")
	writeFile(t, filepath.Join(cwd, "AGENTS.md"), "cwd file")

	got, err := Load(cwd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "cwd file" {
		t.Errorf("Load() = %q, want only cwd's own AGENTS.md (no git root to walk up to)", got)
	}
}

func TestLoadEnforcesPerFileCap(t *testing.T) {
	isolateHome(t)
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "AGENTS.md"), strings.Repeat("x", 9000))

	got, err := Load(cwd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != maxFileBytes {
		t.Errorf("len(Load()) = %d, want %d (per-file cap)", len(got), maxFileBytes)
	}
	if got != strings.Repeat("x", maxFileBytes) {
		t.Error("Load() content mismatch after truncation")
	}
}

func TestLoadEnforcesTotalCap(t *testing.T) {
	_, xdgConfigHome := isolateHome(t)
	writeFile(t, filepath.Join(xdgConfigHome, "liam", "LIAM.md"), strings.Repeat("p", 10000))

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("Mkdir .git: %v", err)
	}
	writeFile(t, filepath.Join(root, "AGENTS.md"), strings.Repeat("a", 10000))
	writeFile(t, filepath.Join(root, "LIAM.md"), strings.Repeat("l", 10000))
	sub := filepath.Join(root, "sub")
	writeFile(t, filepath.Join(sub, "AGENTS.md"), strings.Repeat("b", 10000))

	got, err := Load(sub)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != maxTotalBytes {
		t.Errorf("len(Load()) = %d, want %d (total cap)", len(got), maxTotalBytes)
	}
	if !strings.HasPrefix(got, strings.Repeat("p", maxFileBytes)) {
		t.Error("Load() should start with the (per-file-capped) personal file")
	}
	if !strings.HasSuffix(got, "b") {
		t.Error("Load() should end with a truncated remainder of the last (deepest) file, cut off mid-file by the total cap")
	}
}

func TestLoadSkipsMissingPersonalFile(t *testing.T) {
	isolateHome(t) // XDG_CONFIG_HOME set, but no liam/LIAM.md written under it
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "AGENTS.md"), "cwd file")

	got, err := Load(cwd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "cwd file" {
		t.Errorf("Load() = %q, want %q", got, "cwd file")
	}
}
