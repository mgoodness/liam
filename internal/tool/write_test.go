package tool

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWriteSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectWrite}
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

	wantDiff := "--- " + path + "\n+++ " + path + "\n@@ -0,0 +1 @@\n+hello\n\\ No newline at end of file\n"
	if got.Content != wantDiff {
		t.Errorf("Content = %q, want all-additions diff %q", got.Content, wantDiff)
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

	wantDiff := "--- " + path + "\n+++ " + path + "\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n"
	if got.Content != wantDiff {
		t.Errorf("Content = %q, want diff against prior content %q", got.Content, wantDiff)
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
