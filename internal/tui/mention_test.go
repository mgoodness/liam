package tui

import (
	"context"
	"fmt"
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

// fakeFindSearcher is a tool.FindSearcher stub enough to drive the "@"
// popup end to end without touching the real filesystem searchers. When
// byQuery is set, Find looks the query up there (letting a test simulate a
// backend whose per-query results differ, e.g. a truncated unfiltered
// listing versus a substring-filtered one); otherwise it always returns
// paths regardless of query. queries, when non-nil, records every query
// Find was called with, in call order.
type fakeFindSearcher struct {
	paths   []string
	byQuery map[string][]string
	queries *[]string
}

func (f fakeFindSearcher) Find(_ context.Context, query string) ([]string, int, error) {
	if f.queries != nil {
		*f.queries = append(*f.queries, query)
	}
	if f.byQuery != nil {
		p := f.byQuery[query]
		return p, len(p), nil
	}
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

func TestSelectingMentionInsertsPlainReferenceNoContent(t *testing.T) {
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
	want := "@" + path
	if mm.input.Value() != want {
		t.Errorf("input.Value() = %q, want %q (a bare reference, no inlined content)", mm.input.Value(), want)
	}
	if strings.Contains(mm.input.Value(), "hello world") {
		t.Error("input.Value() contains the file's content, want a plain reference only")
	}
}

func TestSelectingMentionWithRangeSuffixInsertsReferenceWithRange(t *testing.T) {
	// The file is never opened, so its content doesn't need to exist on
	// disk at all — issue #155's point is that selecting a mention no
	// longer reads the file.
	cases := []struct {
		name   string
		suffix string
	}{
		{"single line", ":42"},
		{"line range", ":2-3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "does-not-exist.txt")

			m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{paths: []string{path}})
			next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
			for _, r := range path + tc.suffix {
				next, _ = next.(Model).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}

			next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			mm := next.(Model)

			want := "@" + path + tc.suffix
			if mm.input.Value() != want {
				t.Errorf("input.Value() = %q, want %q", mm.input.Value(), want)
			}
		})
	}
}

func TestMentionPopupAlwaysAsksSearcherForUnfilteredWorkspaceListing(t *testing.T) {
	// The fuzzy-ranking layer lives client-side in the popup, so the
	// backend must always be asked for the full listing (empty query) —
	// pre-filtering server-side by substring alone would hide fuzzy-only
	// matches (issue #155).
	var queries []string
	m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{
		paths:   []string{"internal/tool/search_stdlib.go"},
		queries: &queries,
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	for _, r := range "srchstdlib" {
		next, _ = next.(Model).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	found := false
	for _, q := range queries {
		if q == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("findSearcher.Find queries = %v, want an unfiltered (\"\") request among them", queries)
	}
	mm := next.(Model)
	if len(mm.mention.matches) != 1 || mm.mention.matches[0].path != "internal/tool/search_stdlib.go" {
		t.Errorf("mention.matches = %v, want the fuzzy match for a non-contiguous query", mm.mention.matches)
	}
}

func TestMentionPopupMergesSubstringMatchBeyondUnfilteredListing(t *testing.T) {
	// Simulates a workspace bigger than the backend's own result cap: the
	// unfiltered ("") listing only covers whichever files a tree walk
	// visits first, but a substring query for "target.go" still finds an
	// exact match past that boundary (tool.FindSearcher's own substring
	// filter runs before its cap, per StdlibSearch.Find) — the popup must
	// not lose that match just because it also always asks for the
	// unfiltered listing (issue #155 code review).
	m := New(agent.Loop{}, config.Config{}, nil).WithFindSearcher(fakeFindSearcher{
		byQuery: map[string][]string{
			"":          {"aaa.go", "bbb.go"},
			"target.go": {"deep/dir/target.go"},
		},
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	for _, r := range "target.go" {
		next, _ = next.(Model).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	mm := next.(Model)

	found := false
	for _, mtch := range mm.mention.matches {
		if mtch.path == "deep/dir/target.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("mention.matches = %v, want deep/dir/target.go included via the substring-filtered request", mm.mention.matches)
	}
}

func TestMatchMentionQueryRanksFuzzyMatchesAheadOfLessRelevant(t *testing.T) {
	// "srchstdlib" isn't a contiguous substring of either candidate, so a
	// substring-only matcher would find nothing; sahilm/fuzzy should still
	// surface the more relevant candidate first.
	paths := []string{"internal/other/unrelated.go", "internal/tool/search_stdlib.go"}

	got := matchMentionQuery(paths, "srchstdlib")
	if len(got) == 0 || got[0].path != "internal/tool/search_stdlib.go" {
		t.Errorf("matchMentionQuery(%q) = %v, want the more relevant match ranked first", "srchstdlib", got)
	}
}

func TestMatchMentionQueryCapsAtMaxMentionMatches(t *testing.T) {
	var paths []string
	for i := 0; i < maxMentionMatches+5; i++ {
		paths = append(paths, fmt.Sprintf("file%d.go", i))
	}

	got := matchMentionQuery(paths, "file")
	if len(got) != maxMentionMatches {
		t.Errorf("matchMentionQuery returned %d matches, want capped at %d", len(got), maxMentionMatches)
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
