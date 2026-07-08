package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func callEdit(t *testing.T, path, oldText, newText string) (string, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"path": path, "old_text": oldText, "new_text": newText})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	return Edit(context.Background(), raw)
}

func TestEdit_UniqueMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if _, err := callEdit(t, path, "world", "there"); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if want := "hello there"; string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEdit_ZeroMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	original := "hello world"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if _, err := callEdit(t, path, "missing", "there"); err == nil {
		t.Fatal("expected error for zero matches")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != original {
		t.Errorf("file was modified despite zero-match error: got %q", got)
	}
}

func TestEdit_MultipleMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	original := "foo bar foo"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if _, err := callEdit(t, path, "foo", "baz"); err == nil {
		t.Fatal("expected error for multiple matches")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != original {
		t.Errorf("file was modified despite multiple-match error: got %q", got)
	}
}

func TestEdit_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := callEdit(t, filepath.Join(dir, "nope.txt"), "a", "b"); err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
