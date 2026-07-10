package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func callWrite(t *testing.T, path, content string) (string, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"path": path, "content": content})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	return Write(context.Background(), raw)
}

func TestWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	if _, err := callWrite(t, path, "hello"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestWrite_AutoCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "f.txt")

	if _, err := callWrite(t, path, "nested"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "nested" {
		t.Errorf("got %q, want %q", got, "nested")
	}
}

func TestWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if _, err := callWrite(t, path, "new"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("got %q, want %q", got, "new")
	}
}

func TestWrite_MissingPath(t *testing.T) {
	if _, err := callWrite(t, "", "x"); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestWriteSummarize_ShowsPath(t *testing.T) {
	args, err := json.Marshal(map[string]string{"path": "/tmp/f.txt", "content": "hello"})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	if got, want := writeSummarize(args), "/tmp/f.txt"; got != want {
		t.Errorf("writeSummarize(%s) = %q, want %q", args, got, want)
	}
}
