package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEditSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectWrite}
	if got := (Edit{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestEditRunReplacesUniqueMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "hello world")

	got := (Edit{}).Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "world",
		"new_string": "there",
	})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if body := readFile(t, path); body != "hello there" {
		t.Errorf("file content = %q, want %q", body, "hello there")
	}

	wantDiff := "--- " + path + "\n+++ " + path + "\n@@ -1 +1 @@\n-hello world\n\\ No newline at end of file\n+hello there\n\\ No newline at end of file\n"
	if got.Content != wantDiff {
		t.Errorf("Content = %q, want unified diff %q", got.Content, wantDiff)
	}
}

func TestEditRunNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "hello world")

	got := (Edit{}).Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "not present",
		"new_string": "there",
	})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
	if body := readFile(t, path); body != "hello world" {
		t.Errorf("file content changed to %q, want unchanged", body)
	}
}

func TestEditRunAmbiguousMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "foo foo")

	got := (Edit{}).Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "foo",
		"new_string": "bar",
	})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
	if body := readFile(t, path); body != "foo foo" {
		t.Errorf("file content changed to %q, want unchanged", body)
	}
}

func TestEditRunMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")

	got := (Edit{}).Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "a",
		"new_string": "b",
	})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestEditRunPreservesFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	writeFile(t, path, "echo hello")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	got := (Edit{}).Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "hello",
		"new_string": "there",
	})
	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("file mode = %v, want %v", info.Mode().Perm(), 0o755)
	}
}
