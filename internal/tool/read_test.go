package tool

import (
	"context"
	"path/filepath"
	"strings"
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

// TestReadRunTruncatesLargeFile covers ADR-0005: read's file content is
// capped at outputCap chars, matching web_fetch's own convention.
func TestReadRunTruncatesLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	content := strings.Repeat("line of text\n", 1000) // well over outputCap
	writeFile(t, path, content)

	got := (Read{}).Run(context.Background(), map[string]any{"path": path})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if len(got.Content) >= len(content) {
		t.Errorf("Content len = %d, want truncated well under the raw %d-byte file", len(got.Content), len(content))
	}
	if !strings.Contains(got.Content, "truncated") {
		t.Errorf("Content = %q, want a truncation marker", got.Content)
	}
}

// TestReadRunAtCapReturnsUnchanged covers the exactly-at-cap boundary at
// the tool level (not just truncate() in isolation, per issue #85's
// acceptance criteria): a file of exactly outputCap bytes must come back
// with no marker and no bytes dropped.
func TestReadRunAtCapReturnsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exact.txt")
	content := strings.Repeat("a", outputCap)
	writeFile(t, path, content)

	got := (Read{}).Run(context.Background(), map[string]any{"path": path})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if got.Content != content {
		t.Errorf("Content len = %d, want exactly outputCap (%d) bytes unchanged", len(got.Content), outputCap)
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
