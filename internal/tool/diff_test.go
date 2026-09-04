package tool

import (
	"strconv"
	"strings"
	"testing"
)

func TestFormatDiffExactOutput(t *testing.T) {
	cases := []struct {
		name     string
		old, new string
		want     string
	}{
		{
			name: "no change",
			old:  "same\n",
			new:  "same\n",
			want: "",
		},
		{
			name: "single hunk",
			old:  "one\ntwo\nthree\n",
			new:  "one\nTWO\nthree\n",
			want: "--- a.txt\n+++ a.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n",
		},
		{
			name: "new file",
			old:  "",
			new:  "one\ntwo\n",
			want: "--- a.txt\n+++ a.txt\n@@ -0,0 +1,2 @@\n+one\n+two\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDiff("a.txt", tc.old, tc.new); got != tc.want {
				t.Errorf("formatDiff() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatDiffMultiHunk(t *testing.T) {
	// Two changes far enough apart (more than 2*DefaultContextLines lines of
	// unchanged content between them) that they render as separate hunks
	// rather than merging into one.
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line" + strconv.Itoa(i)
	}
	old := strings.Join(lines, "\n") + "\n"

	changed := append([]string(nil), lines...)
	changed[1] = "CHANGED-NEAR-TOP"
	changed[18] = "CHANGED-NEAR-BOTTOM"
	newContent := strings.Join(changed, "\n") + "\n"

	got := formatDiff("a.txt", old, newContent)

	hunkCount := strings.Count(got, "@@ -")
	if hunkCount != 2 {
		t.Fatalf("formatDiff() produced %d hunks, want 2:\n%s", hunkCount, got)
	}
	if !strings.Contains(got, "-line1\n+CHANGED-NEAR-TOP") {
		t.Errorf("formatDiff() missing top change:\n%s", got)
	}
	if !strings.Contains(got, "-line18\n+CHANGED-NEAR-BOTTOM") {
		t.Errorf("formatDiff() missing bottom change:\n%s", got)
	}
}

func TestFormatDiffTruncatesAtHunkBoundary(t *testing.T) {
	// Build enough widely-separated single-line changes that the total
	// hunk-line count exceeds diffLineCap, forcing truncation.
	const numChanges = 100
	lines := make([]string, numChanges*10)
	for i := range lines {
		lines[i] = "line" + strconv.Itoa(i)
	}
	old := strings.Join(lines, "\n") + "\n"

	changed := append([]string(nil), lines...)
	for i := 0; i < numChanges; i++ {
		changed[i*10] = "CHANGED" + strconv.Itoa(i)
	}
	newContent := strings.Join(changed, "\n") + "\n"

	got := formatDiff("a.txt", old, newContent)

	if !strings.Contains(got, "more lines truncated") {
		t.Fatalf("formatDiff() didn't truncate a large multi-hunk diff:\n%s", got)
	}

	marker := "\n[... "
	idx := strings.Index(got, marker)
	if idx < 0 {
		t.Fatalf("formatDiff() missing truncation marker:\n%s", got)
	}
	kept := got[:idx]
	if strings.Count(kept, "@@ -") == 0 {
		t.Fatalf("formatDiff() truncated with no hunks kept:\n%s", got)
	}
	// A cut landing mid-hunk would leave the kept portion ending on a bare
	// "-"/"+"/" " prefixed line with no trailing newline consumed yet; a
	// hunk-boundary cut always ends on a full line.
	if !strings.HasSuffix(kept, "\n") {
		t.Errorf("formatDiff() truncation didn't land on a line boundary: %q", kept)
	}
}
