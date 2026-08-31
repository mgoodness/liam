package tool

import (
	"context"
	"testing"
)

func TestStdlibSearchGrep(t *testing.T) {
	s := StdlibSearch{Dir: "testdata/fixture_grep"}

	matches, total, err := s.Grep(context.Background(), "foo")
	if err != nil {
		t.Fatalf("Grep() err = %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	want := []GrepMatch{
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

func TestStdlibSearchGrepInvalidRegex(t *testing.T) {
	s := StdlibSearch{Dir: "testdata/fixture_grep"}

	_, _, err := s.Grep(context.Background(), "(unclosed")
	if err == nil {
		t.Fatal("Grep() err = nil, want an error for an invalid regular expression")
	}
}

func TestStdlibSearchGrepNoMatches(t *testing.T) {
	s := StdlibSearch{Dir: "testdata/fixture_grep"}

	matches, total, err := s.Grep(context.Background(), "doesnotexistanywhere")
	if err != nil {
		t.Fatalf("Grep() err = %v", err)
	}
	if total != 0 || len(matches) != 0 {
		t.Errorf("total = %d, len(matches) = %d, want 0, 0", total, len(matches))
	}
}

func TestStdlibSearchFind(t *testing.T) {
	s := StdlibSearch{Dir: "testdata/fixture_find"}

	paths, total, err := s.Find(context.Background(), "apple")
	if err != nil {
		t.Fatalf("Find() err = %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	want := []string{"apple.go", "sub/apple_pie.go"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestStdlibSearchFindEmptyQueryListsEverything(t *testing.T) {
	s := StdlibSearch{Dir: "testdata/fixture_find"}

	paths, total, err := s.Find(context.Background(), "")
	if err != nil {
		t.Fatalf("Find() err = %v", err)
	}
	if total != 3 || len(paths) != 3 {
		t.Errorf("total = %d, len(paths) = %d, want 3, 3", total, len(paths))
	}
}

// TestGrepRunMatchesGoldenOutput and TestFindRunMatchesGoldenOutput are
// this ticket's golden-file coverage (issue #49's acceptance criteria):
// the same fixture inputs, run through the stdlib searcher here and the
// fff-mcp searcher in internal/mcp's own fff_test.go, must produce
// byte-identical Result.Content against the shared golden file.
func TestGrepRunMatchesGoldenOutput(t *testing.T) {
	searcher := StdlibSearch{Dir: "testdata/fixture_grep"}
	got := Grep{Searcher: searcher}.Run(context.Background(), map[string]any{"query": "foo"})
	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}

	want := readFile(t, "testdata/grep_foo.golden")
	if got.Content != want {
		t.Errorf("Content = %q, want golden %q", got.Content, want)
	}
}

func TestFindRunMatchesGoldenOutput(t *testing.T) {
	searcher := StdlibSearch{Dir: "testdata/fixture_find"}
	got := Find{Searcher: searcher}.Run(context.Background(), map[string]any{"query": "apple"})
	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}

	want := readFile(t, "testdata/find_apple.golden")
	if got.Content != want {
		t.Errorf("Content = %q, want golden %q", got.Content, want)
	}
}
