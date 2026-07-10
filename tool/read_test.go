package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func callRead(t *testing.T, path string, offset, limit int) (string, error) {
	t.Helper()
	args := map[string]any{"path": path}
	if offset != 0 {
		args["offset"] = offset
	}
	if limit != 0 {
		args["limit"] = limit
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	return Read(context.Background(), raw)
}

func TestRead_FullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	want := "line1\nline2\nline3"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := callRead(t, path, 0, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRead_OffsetLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	lines := []string{"a", "b", "c", "d", "e"}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := callRead(t, path, 1, 2)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if want := "b\nc"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRead_OffsetBeyondEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("a\nb"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := callRead(t, path, 10, 5)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestRead_LargeFileSlicing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	const total = 10000
	lines := make([]string, total)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := callRead(t, path, 5000, 3)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if want := strings.Join(lines[5000:5003], "\n"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRead_MissingPath(t *testing.T) {
	if _, err := callRead(t, "", 0, 0); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestRead_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := callRead(t, filepath.Join(dir, "nope.txt"), 0, 0); err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadSummarize_ShowsPath(t *testing.T) {
	args, err := json.Marshal(map[string]string{"path": "/tmp/f.txt"})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	if got, want := readSummarize(args), "/tmp/f.txt"; got != want {
		t.Errorf("readSummarize(%s) = %q, want %q", args, got, want)
	}
}
