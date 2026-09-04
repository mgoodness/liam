package tool

import (
	"fmt"
	"strings"

	"github.com/aymanbagabas/go-udiff"
)

// diffLineCap caps how many hunk lines (excluding the "---"/"+++"/"@@"
// headers) formatDiff keeps before truncating, playing the same role
// outputCap plays for plain-text tool output — but counted in diff lines
// and cut at a hunk boundary rather than mid-hunk (ADR-0015), since a
// byte-count cutoff can slice a hunk in half, which reads as corrupted
// rather than merely abbreviated.
const diffLineCap = 300

// formatDiff renders a unified diff between old and newContent, labeled
// path in both the "---" and "+++" headers. old == newContent (no change)
// renders as the empty string, matching udiff.Unified's own convention;
// old == "" (a brand-new file) renders as an all-additions diff with no
// special-cased "new file" message, since a unified diff against empty
// content already reads correctly on its own.
func formatDiff(path, old, newContent string) string {
	edits := udiff.Lines(old, newContent)
	// edits come from udiff.Lines against old itself, so they're always
	// internally consistent — ToUnifiedDiff's error return can't fire.
	ud, _ := udiff.ToUnifiedDiff(path, path, old, edits, udiff.DefaultContextLines)
	if len(ud.Hunks) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", ud.From, ud.To)

	total := 0
	for _, h := range ud.Hunks {
		total += len(h.Lines)
	}

	kept := 0
	for _, h := range ud.Hunks {
		if kept+len(h.Lines) > diffLineCap {
			fmt.Fprintf(&b, "\n[... %d more lines truncated ...]\n", total-kept)
			break
		}
		writeHunk(&b, h)
		kept += len(h.Lines)
	}
	return b.String()
}

// writeHunk renders one hunk in standard unified-diff form: an
// "@@ -from,count +from,count @@" header sized to its delete/context and
// insert/context line counts, followed by its lines prefixed "-"/"+"/" ".
func writeHunk(b *strings.Builder, h *udiff.Hunk) {
	fromCount, toCount := 0, 0
	for _, l := range h.Lines {
		switch l.Kind {
		case udiff.Delete:
			fromCount++
		case udiff.Insert:
			toCount++
		default:
			fromCount++
			toCount++
		}
	}

	fmt.Fprintf(b, "@@ -%s +%s @@\n", formatRange(h.FromLine, fromCount), formatRange(h.ToLine, toCount))

	for _, l := range h.Lines {
		switch l.Kind {
		case udiff.Delete:
			fmt.Fprintf(b, "-%s", l.Content)
		case udiff.Insert:
			fmt.Fprintf(b, "+%s", l.Content)
		default:
			fmt.Fprintf(b, " %s", l.Content)
		}
		if !strings.HasSuffix(l.Content, "\n") {
			fmt.Fprint(b, "\n\\ No newline at end of file\n")
		}
	}
}

// formatRange renders one side of a hunk header's "-from,count"/"+from,count"
// span (without the leading sign), collapsing to a bare line number when
// count is 1 and special-casing "0,0" to match GNU diff -u's convention for
// a brand-new (empty) file.
func formatRange(line, count int) string {
	switch {
	case count > 1:
		return fmt.Sprintf("%d,%d", line, count)
	case line == 1 && count == 0:
		return "0,0"
	default:
		return fmt.Sprintf("%d", line)
	}
}
