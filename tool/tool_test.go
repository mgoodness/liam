package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCall_UnknownTool(t *testing.T) {
	if _, err := Call(context.Background(), "nope", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestCall_Dispatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	writeArgs, err := json.Marshal(map[string]string{"path": path, "content": "hi"})
	if err != nil {
		t.Fatalf("marshaling write args: %v", err)
	}
	if _, err := Call(context.Background(), "write", writeArgs); err != nil {
		t.Fatalf("write via Call: %v", err)
	}

	readArgs, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("marshaling read args: %v", err)
	}
	got, err := Call(context.Background(), "read", readArgs)
	if err != nil {
		t.Fatalf("read via Call: %v", err)
	}
	if got != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

func TestTruncate(t *testing.T) {
	small := "hello"
	if got := truncate(small); got != small {
		t.Errorf("small input should be unchanged, got %q", got)
	}

	big := strings.Repeat("a", MaxResultBytes+100)
	got := truncate(big)
	if !strings.HasPrefix(got, strings.Repeat("a", MaxResultBytes)) {
		t.Error("truncated output should preserve the first MaxResultBytes bytes")
	}
	if !strings.Contains(got, "[truncated: 100 bytes omitted]") {
		t.Errorf("expected truncation marker, got tail %q", got[len(got)-40:])
	}
}
