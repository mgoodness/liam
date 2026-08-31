package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
)

func TestParseFileReferenceNoSuffixIsNoRange(t *testing.T) {
	ref := parseFileReference("internal/tui/tui.go")
	if ref.hasRange {
		t.Fatalf("hasRange = true, want false for a plain path")
	}
	if ref.query != "internal/tui/tui.go" || ref.start != 0 || ref.end != 0 {
		t.Errorf("got %+v, want the path unchanged with a zero range", ref)
	}
}

func TestParseFileReferenceSingleLine(t *testing.T) {
	ref := parseFileReference("main.go:42")
	if !ref.hasRange {
		t.Fatal("hasRange = false, want true for \"main.go:42\"")
	}
	if ref.query != "main.go" || ref.start != 42 || ref.end != 42 {
		t.Errorf("got %+v, want query=\"main.go\", start=42, end=42", ref)
	}
}

func TestParseFileReferenceLineRange(t *testing.T) {
	ref := parseFileReference("main.go:10-20")
	if !ref.hasRange {
		t.Fatal("hasRange = false, want true for \"main.go:10-20\"")
	}
	if ref.query != "main.go" || ref.start != 10 || ref.end != 20 {
		t.Errorf("got %+v, want query=\"main.go\", start=10, end=20", ref)
	}
}

func TestParseFileReferenceRejectsDescendingRange(t *testing.T) {
	ref := parseFileReference("main.go:20-10")
	if ref.hasRange {
		t.Fatal("hasRange = true, want false for a descending range")
	}
	if ref.query != "main.go:20-10" {
		t.Errorf("query = %q, want the original query returned unchanged", ref.query)
	}
}

func TestParseFileReferenceRejectsZeroLine(t *testing.T) {
	if parseFileReference("main.go:0").hasRange {
		t.Error("hasRange = true, want false for line 0")
	}
}

func TestParseFileReferencePathWithColonNoDigits(t *testing.T) {
	// A path that happens to contain ":" but no trailing digits (e.g. a
	// Windows-style drive prefix) must not be misparsed as a line range.
	ref := parseFileReference("C:/foo/bar.go")
	if ref.hasRange {
		t.Error("hasRange = true, want false when the suffix isn't numeric")
	}
	if ref.query != "C:/foo/bar.go" {
		t.Errorf("query = %q, want the query unchanged", ref.query)
	}
}

func TestFindMentionStartRequiresPrecedingWhitespace(t *testing.T) {
	line := []rune("email@example.com")
	if _, ok := findMentionStart(line, len(line)); ok {
		t.Error("findMentionStart found a token in an email address, want none")
	}
}

func TestFindMentionStartAtLineStart(t *testing.T) {
	line := []rune("@main.go")
	start, ok := findMentionStart(line, len(line))
	if !ok || start != 0 {
		t.Errorf("findMentionStart = %d, %v, want 0, true", start, ok)
	}
}

func TestFindMentionStartAfterWhitespace(t *testing.T) {
	line := []rune("read @main.go please")
	start, ok := findMentionStart(line, len("read @main.go"))
	if !ok || start != 5 {
		t.Errorf("findMentionStart = %d, %v, want 5, true", start, ok)
	}
}

func TestFindMentionStartClosesOnWhitespace(t *testing.T) {
	line := []rune("@main.go please")
	// Cursor sits inside "please", past the whitespace that follows the
	// mention token — no longer an unbroken "@query".
	if _, ok := findMentionStart(line, len(line)); ok {
		t.Error("findMentionStart found a token past whitespace, want none")
	}
}

func TestRenderFileReferenceWholeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "greeting.txt")
	if err := os.WriteFile(path, []byte("hello\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := renderFileReference(path, fileReference{})
	if err != nil {
		t.Fatalf("renderFileReference: %v", err)
	}
	want := "[file: " + path + "]\nhello\nworld\n[/file: " + path + "]"
	if got != want {
		t.Errorf("renderFileReference = %q, want %q", got, want)
	}
}

func TestRenderFileReferenceLineRangeAddsLineNumberContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "code.go")
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := renderFileReference(path, fileReference{start: 2, end: 4, hasRange: true})
	if err != nil {
		t.Fatalf("renderFileReference: %v", err)
	}
	if !strings.Contains(got, "Line 2: line2") || !strings.Contains(got, "Line 4: line4") {
		t.Errorf("renderFileReference = %q, want line-numbered rows for lines 2-4", got)
	}
	if strings.Contains(got, "line1") || strings.Contains(got, "line5") {
		t.Errorf("renderFileReference = %q, want only lines 2-4 inlined", got)
	}
}

func TestRenderFileReferenceOutOfBoundsRangeErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "short.txt")
	if err := os.WriteFile(path, []byte("only one line"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := renderFileReference(path, fileReference{start: 10, end: 20, hasRange: true}); err == nil {
		t.Error("renderFileReference with an out-of-bounds range returned nil error")
	}
}

// fakeFindSearcher is a tool.FindSearcher stub returning a fixed candidate
// list regardless of query, enough to drive the "@" popup end to end
// without touching the real filesystem searchers.
type fakeFindSearcher struct {
	paths []string
}

func (f fakeFindSearcher) Find(_ context.Context, _ string) ([]string, int, error) {
	return f.paths, len(f.paths), nil
}

func TestTypingAtOpensMentionPopup(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{paths: []string{"main.go", "internal/tui/tui.go"}})

	next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	mm := next.(Model)

	if !mm.mention.active {
		t.Fatal("mention.active = false after typing \"@\", want true")
	}
	if len(mm.mention.matches) != 2 {
		t.Errorf("mention.matches = %v, want 2 candidates", mm.mention.matches)
	}
}

func TestMentionClosesWhenTokenBreaksOnWhitespace(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{paths: []string{"main.go"}})

	next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	mm := next.(Model)

	if mm.mention.active {
		t.Error("mention.active = true after a space, want false")
	}
}

func TestSelectingMentionInlinesFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{paths: []string{path}})
	next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})

	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	mm := next.(Model)

	if mm.mention.active {
		t.Error("mention.active = true after selecting, want false")
	}
	want := "[file: " + path + "]\nhello world\n[/file: " + path + "]"
	if mm.input.Value() != want {
		t.Errorf("input.Value() = %q, want %q", mm.input.Value(), want)
	}
}

func TestSelectingMentionWithLineRangeInlinesOnlyThoseLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	content := "one\ntwo\nthree\nfour"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{paths: []string{path}})
	next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	for _, r := range path + ":2-3" {
		next, _ = next.(Model).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm := next.(Model)

	val := mm.input.Value()
	if !strings.Contains(val, "Line 2: two") || !strings.Contains(val, "Line 3: three") {
		t.Fatalf("input.Value() = %q, want lines 2-3 inlined with line numbers", val)
	}
	if strings.Contains(val, "one") || strings.Contains(val, "four") {
		t.Errorf("input.Value() = %q, want only lines 2-3", val)
	}
}

func TestArrowKeysMoveMentionSelectionNotHistory(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{paths: []string{"a.go", "b.go"}})
	m.hist.add("earlier message")

	next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mm := next.(Model)

	if mm.mention.selected != 1 {
		t.Errorf("mention.selected = %d, want 1 (moved within the popup)", mm.mention.selected)
	}
	if mm.input.Value() != "@" {
		t.Errorf("input.Value() = %q, want \"@\" (history must not have been recalled)", mm.input.Value())
	}
}
