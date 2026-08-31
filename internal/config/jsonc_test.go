package config

import "testing"

func TestStripCommentsLineComment(t *testing.T) {
	in := "{\n  \"a\": 1, // trailing comment\n  \"b\": 2\n}\n"
	got := stripComments([]byte(in))
	want := map[string]any{"a": 1.0, "b": 2.0}
	assertJSONEqual(t, got, want)
}

func TestStripCommentsBlockComment(t *testing.T) {
	in := `{
  /* leading block
     comment */
  "a": 1 /* inline */, "b": 2
}`
	got := stripComments([]byte(in))
	want := map[string]any{"a": 1.0, "b": 2.0}
	assertJSONEqual(t, got, want)
}

func TestStripCommentsIgnoresSlashesInsideStrings(t *testing.T) {
	in := `{"path": "https://example.com", "note": "not a /* comment */"}`
	got := stripComments([]byte(in))
	want := map[string]any{
		"path": "https://example.com",
		"note": "not a /* comment */",
	}
	assertJSONEqual(t, got, want)
}

func TestStripCommentsHandlesEscapedQuotes(t *testing.T) {
	in := `{"note": "she said \"// not a comment\""}`
	got := stripComments([]byte(in))
	want := map[string]any{"note": `she said "// not a comment"`}
	assertJSONEqual(t, got, want)
}

func TestStripCommentsNoTrailingNewline(t *testing.T) {
	in := `{"a": 1} // trailing, no newline at EOF`
	got := stripComments([]byte(in))
	want := map[string]any{"a": 1.0}
	assertJSONEqual(t, got, want)
}
