package tool

import "testing"

func TestFormatGrepResultsNoMatches(t *testing.T) {
	got := formatGrepResults(nil, 0)
	want := "Found 0 matches."
	if got != want {
		t.Errorf("formatGrepResults(nil, 0) = %q, want %q", got, want)
	}
}

func TestFormatGrepResultsSingularMatch(t *testing.T) {
	matches := []GrepMatch{{File: "a.go", Line: 3, Text: "foo bar"}}
	got := formatGrepResults(matches, 1)
	want := "Found 1 match\n\na.go\nLine 3: foo bar"
	if got != want {
		t.Errorf("formatGrepResults() = %q, want %q", got, want)
	}
}

func TestFormatGrepResultsGroupsByFile(t *testing.T) {
	matches := []GrepMatch{
		{File: "a.go", Line: 3, Text: "foo bar"},
		{File: "a.go", Line: 5, Text: "baz foo"},
		{File: "sub/b.go", Line: 3, Text: "func foo() {}"},
	}
	got := formatGrepResults(matches, 3)
	want := "Found 3 matches\n\na.go\nLine 3: foo bar\nLine 5: baz foo\n\nsub/b.go\nLine 3: func foo() {}"
	if got != want {
		t.Errorf("formatGrepResults() = %q, want %q", got, want)
	}
}

func TestFormatGrepResultsTruncationNotice(t *testing.T) {
	matches := []GrepMatch{{File: "a.go", Line: 1, Text: "foo"}}
	got := formatGrepResults(matches, 5)
	want := "Found 5 matches (more matches available)\n\na.go\nLine 1: foo\n\n[... truncated, 4 more matches not shown ...]"
	if got != want {
		t.Errorf("formatGrepResults() = %q, want %q", got, want)
	}
}

func TestFormatGrepResultsTruncationNoticeSingular(t *testing.T) {
	matches := []GrepMatch{{File: "a.go", Line: 1, Text: "foo"}}
	got := formatGrepResults(matches, 2)
	want := "Found 2 matches (more matches available)\n\na.go\nLine 1: foo\n\n[... truncated, 1 more match not shown ...]"
	if got != want {
		t.Errorf("formatGrepResults() = %q, want %q", got, want)
	}
}

func TestFormatFindResultsNoResults(t *testing.T) {
	got := formatFindResults(nil, 0)
	want := "Found 0 files."
	if got != want {
		t.Errorf("formatFindResults(nil, 0) = %q, want %q", got, want)
	}
}

func TestFormatFindResultsSingularFile(t *testing.T) {
	got := formatFindResults([]string{"a.go"}, 1)
	want := "Found 1 file\n\na.go"
	if got != want {
		t.Errorf("formatFindResults() = %q, want %q", got, want)
	}
}

func TestFormatFindResultsTruncationNotice(t *testing.T) {
	got := formatFindResults([]string{"a.go"}, 3)
	want := "Found 3 files (more files available)\n\na.go\n\n[... truncated, 2 more files not shown ...]"
	if got != want {
		t.Errorf("formatFindResults() = %q, want %q", got, want)
	}
}
