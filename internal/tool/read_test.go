package tool

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReadSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectRead}
	if got := (Read{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestReadRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "hello world")

	got := (Read{}).Run(context.Background(), map[string]any{"path": path})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if got.Content != "hello world" {
		t.Errorf("Content = %q, want %q", got.Content, "hello world")
	}
}

func TestReadRunMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")

	got := (Read{}).Run(context.Background(), map[string]any{"path": path})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true; Content = %q", got.Content)
	}
}

func TestReadRunMissingPathArg(t *testing.T) {
	got := (Read{}).Run(context.Background(), map[string]any{})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}
