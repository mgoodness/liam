package mcp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/tool"
)

// The raw text fixtures below are captured verbatim from a real fff-mcp
// (v0.x, installed via Homebrew) run against this repo — see ticket #49.

func TestParseFFFGrepOutputUncapped(t *testing.T) {
	raw := "a.go\n 3: foo bar\n 5: baz foo\nsub/b.go\n 3: func foo() {}"

	matches, total := parseFFFGrepOutput(raw)

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	want := []tool.GrepMatch{
		{File: "a.go", Line: 3, Text: "foo bar"},
		{File: "a.go", Line: 5, Text: "baz foo"},
		{File: "sub/b.go", Line: 3, Text: "func foo() {}"},
	}
	if len(matches) != len(want) {
		t.Fatalf("matches = %+v, want %+v", matches, want)
	}
	for i := range want {
		if matches[i] != want[i] {
			t.Errorf("matches[%d] = %+v, want %+v", i, matches[i], want[i])
		}
	}
}

func TestParseFFFGrepOutputCappedWithHintAndCursor(t *testing.T) {
	raw := "→ Read cmd/liam/main_test.go (only match)\n3/28 matches shown\ncmd/liam/main_test.go\n 140: loop := agent.Loop{Provider: fp, Tools: tool.NewRegistry()}\n 156: loop := agent.Loop{Provider: doneProvider{}, Tools: tool.NewRegistry()}\n 173: loop := agent.Loop{Provider: doneProvider{}, Tools: tool.NewRegistry()}\ncursor: 3"

	matches, total := parseFFFGrepOutput(raw)

	if total != 28 {
		t.Errorf("total = %d, want 28 (parsed from the header, not len(matches))", total)
	}
	if len(matches) != 3 {
		t.Fatalf("len(matches) = %d, want 3: %+v", len(matches), matches)
	}
	if matches[0] != (tool.GrepMatch{File: "cmd/liam/main_test.go", Line: 140, Text: "loop := agent.Loop{Provider: fp, Tools: tool.NewRegistry()}"}) {
		t.Errorf("matches[0] = %+v", matches[0])
	}
}

func TestParseFFFGrepOutputNoMatches(t *testing.T) {
	matches, total := parseFFFGrepOutput("0 matches.")

	if total != 0 || matches != nil {
		t.Errorf("total = %d, matches = %+v, want 0, nil", total, matches)
	}
}

func TestParseFFFFindOutputUncapped(t *testing.T) {
	raw := "internal/tool/registry.go git:clean\ninternal/tool/registry_test.go git:clean\ndocs/adr/0002-hooks-fail-open-on-infrastructure-failure.md git:clean"

	paths, total := parseFFFFindOutput(raw)

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	want := []string{
		"internal/tool/registry.go",
		"internal/tool/registry_test.go",
		"docs/adr/0002-hooks-fail-open-on-infrastructure-failure.md",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestParseFFFFindOutputCappedWithCursor(t *testing.T) {
	raw := "2/106 matches\ndocs/adr/0005-bash-read-output-truncation.md git:clean\ndocs/research/cutting-agent-token-usage.md git:clean\ncursor: 1"

	paths, total := parseFFFFindOutput(raw)

	if total != 106 {
		t.Errorf("total = %d, want 106", total)
	}
	want := []string{"docs/adr/0005-bash-read-output-truncation.md", "docs/research/cutting-agent-token-usage.md"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestParseFFFFindOutputNoMatches(t *testing.T) {
	paths, total := parseFFFFindOutput("0 results (106 indexed)")

	if total != 0 || paths != nil {
		t.Errorf("total = %d, paths = %v, want 0, nil", total, paths)
	}
}

// fffGrepHandler and fffFindHandler stand in for a real fff-mcp process,
// returning the exact literal text a real fff-mcp prints for these two
// fixture queries — the same fixtures internal/tool/search_stdlib_test.go
// runs the stdlib searcher against.
func fffGrepHandler(_ context.Context, _ *sdkmcp.CallToolRequest, _ map[string]any) (*sdkmcp.CallToolResult, any, error) {
	text := "a.go\n 3: foo bar\n 5: baz foo\nsub/b.go\n 3: func foo() {}"
	return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}}}, nil, nil
}

func fffFindHandler(_ context.Context, _ *sdkmcp.CallToolRequest, _ map[string]any) (*sdkmcp.CallToolResult, any, error) {
	text := "apple.go git:clean\nsub/apple_pie.go git:clean"
	return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}}}, nil, nil
}

// TestFFFGrepRunMatchesGoldenOutput and TestFFFFindRunMatchesGoldenOutput
// are this ticket's golden-file coverage (issue #49's acceptance
// criteria): the fff-mcp searcher here and the stdlib searcher in
// internal/tool's own search_stdlib_test.go, run against the same fixture
// inputs, must produce byte-identical Result.Content — the same golden
// file both tests read from internal/tool/testdata.
func TestFFFGrepRunMatchesGoldenOutput(t *testing.T) {
	session, _ := connectStub(t, func(s *sdkmcp.Server) {
		sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "grep", Description: "grep"}, fffGrepHandler)
	})
	searcher := &FFF{session: session}

	got := tool.Grep{Searcher: searcher}.Run(context.Background(), map[string]any{"query": "foo"})
	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}

	want := readGolden(t, "grep_foo.golden")
	if got.Content != want {
		t.Errorf("Content = %q, want golden %q", got.Content, want)
	}
}

func TestFFFFindRunMatchesGoldenOutput(t *testing.T) {
	session, _ := connectStub(t, func(s *sdkmcp.Server) {
		sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "find_files", Description: "find_files"}, fffFindHandler)
	})
	searcher := &FFF{session: session}

	got := tool.Find{Searcher: searcher}.Run(context.Background(), map[string]any{"query": "apple"})
	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}

	want := readGolden(t, "find_apple.golden")
	if got.Content != want {
		t.Errorf("Content = %q, want golden %q", got.Content, want)
	}
}

// readGolden reads name from internal/tool/testdata — the same golden
// fixtures internal/tool's own stdlib-searcher tests read directly.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "tool", "testdata", name))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", name, err)
	}
	return string(data)
}

func TestDetectFFFFindsBinaryOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable bit not portable to windows")
	}

	dir := t.TempDir()
	fake := filepath.Join(dir, fffMCPCommand)
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", dir)

	if !DetectFFF() {
		t.Error("DetectFFF() = false, want true with a fake fff-mcp on $PATH")
	}
}

func TestDetectFFFAbsentFromPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if DetectFFF() {
		t.Error("DetectFFF() = true, want false with an empty $PATH")
	}
}

func TestStartFFFConnectError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // fffMCPCommand isn't resolvable

	_, err := StartFFF(context.Background(), ".")
	if err == nil {
		t.Fatal("StartFFF() error = nil, want a connect error")
	}
}
