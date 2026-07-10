package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCall_UnknownTool(t *testing.T) {
	if _, err := Call(context.Background(), Tools, "nope", json.RawMessage(`{}`)); err == nil {
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
	if _, err := Call(context.Background(), Tools, "write", writeArgs); err != nil {
		t.Fatalf("write via Call: %v", err)
	}

	readArgs, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("marshaling read args: %v", err)
	}
	got, err := Call(context.Background(), Tools, "read", readArgs)
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

func TestNew_NoBraveAPIKey_OmitsWebSearch(t *testing.T) {
	tools := New("")

	if _, ok := tools["web_search"]; ok {
		t.Error("New(\"\") includes web_search, want it absent with no Brave API key")
	}
	for _, name := range []string{"read", "write", "edit", "bash"} {
		if _, ok := tools[name]; !ok {
			t.Errorf("New(\"\") missing core tool %q", name)
		}
	}
}

func TestNew_WithBraveAPIKey_IncludesWorkingWebSearch(t *testing.T) {
	tools := New("test-api-key")

	got, ok := tools["web_search"]
	if !ok {
		t.Fatal("New(\"test-api-key\") missing web_search, want it present")
	}
	if got.Definition.Name != "web_search" {
		t.Errorf("web_search Definition.Name = %q, want %q", got.Definition.Name, "web_search")
	}
	if got.Handler == nil {
		t.Error("web_search Handler is nil, want a working handler")
	}
}

func TestCall_DispatchesWebSearchFromNewSet(t *testing.T) {
	srv := newTestWebSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(braveSearchResponse{})
	})

	tools := New("test-api-key")
	tools["web_search"] = NewWebSearchTool("test-api-key", srv.URL)

	args, err := json.Marshal(map[string]string{"query": "golang"})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	if _, err := Call(context.Background(), tools, "web_search", args); err != nil {
		t.Fatalf("web_search via Call: %v", err)
	}
}

func TestDefinitions_SortedByName(t *testing.T) {
	tools := New("test-api-key")
	defs := Definitions(tools)

	if len(defs) != len(tools) {
		t.Fatalf("Definitions returned %d entries, want %d", len(defs), len(tools))
	}
	for i := 1; i < len(defs); i++ {
		if defs[i-1].Name >= defs[i].Name {
			t.Errorf("Definitions not sorted: %q before %q", defs[i-1].Name, defs[i].Name)
		}
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
