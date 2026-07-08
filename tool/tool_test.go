package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestTruncate_DoesNotSplitMultibyteRune(t *testing.T) {
	// Place a 4-byte emoji straddling the byte cap so a naive s[:MaxResultBytes]
	// cut would slice it in half and produce invalid UTF-8.
	prefix := strings.Repeat("a", MaxResultBytes-1)
	s := prefix + "🎉" + "trailing"

	got := truncate(s)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated output is not valid UTF-8: %q", got)
	}
	if strings.Contains(got, "🎉") {
		t.Error("expected the straddling rune to be excluded, not partially included")
	}
	if !strings.Contains(got, "bytes omitted]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
}
