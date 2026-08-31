package tool

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWriteSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectWrite, Permission: PermissionAllow}
	if got := (Write{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestWriteRunCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")

	got := (Write{}).Run(context.Background(), map[string]any{
		"path":    path,
		"content": "hello",
	})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if body := readFile(t, path); body != "hello" {
		t.Errorf("file content = %q, want %q", body, "hello")
	}
}

func TestWriteRunOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "old")

	got := (Write{}).Run(context.Background(), map[string]any{
		"path":    path,
		"content": "new",
	})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if body := readFile(t, path); body != "new" {
		t.Errorf("file content = %q, want %q", body, "new")
	}
}

func TestWriteRunMissingContentArg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")

	got := (Write{}).Run(context.Background(), map[string]any{"path": path})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestWriteRunInvalidPath(t *testing.T) {
	dir := t.TempDir()
	// The parent directory doesn't exist, so the write must fail.
	path := filepath.Join(dir, "nonexistent-subdir", "a.txt")

	got := (Write{}).Run(context.Background(), map[string]any{
		"path":    path,
		"content": "hello",
	})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}
