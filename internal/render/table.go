package render

// TruncateWidth hard-truncates s to at most n runes, replacing the last
// rune with an ellipsis when it would otherwise overflow — the same
// truncate-not-wrap convention ToolCall's result-line summarization and
// statusline.Render's row truncation already use, shared here so the
// slash-command popup and /skills' output don't each reinvent it. n <= 0
// means there's no room at all, so it returns "".
func TruncateWidth(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ColumnWidth returns the width a table's left-hand column should use to
// fit every one of labels without wrapping: the longest label, capped at
// max so a single outlier can't blow out the whole table and starve
// whatever column follows it. max <= 0 means uncapped, matching
// statusline.truncateRows' "no width, no limit" convention.
func ColumnWidth(labels []string, max int) int {
	w := 0
	for _, l := range labels {
		if n := len([]rune(l)); n > w {
			w = n
		}
	}
	if max > 0 && w > max {
		return max
	}
	return w
}

// MinDescWidth is the floor a two-column "label — description" table holds
// its description column to even on a narrow display, so a long label
// can't shrink it to nothing.
const MinDescWidth = 10

// TableColumns computes the label/description column widths for a
// "label — description" table (the /-command popup and /skills' output
// both render one) that must fit within a total display width: labelWidth
// is the longest of labels, capped just enough that descWidth — whatever's
// left after labelWidth and overhead, the combined rune width of every
// fixed character surrounding the two columns (a row prefix, the
// separator between columns) — never drops below minDesc. Centralized here
// instead of duplicated per caller because the arithmetic (cap the label,
// floor the description, both against the same width budget) is identical
// either way; only the overhead and minDesc callers pass in differ.
//
// A width so narrow that even minDesc doesn't fit still returns exactly
// minDesc for descWidth — the row will overflow on a genuinely tiny
// display, which is an accepted edge case (see issue #148's "use
// judgment") rather than something worth its own truncation-of-truncation
// handling.
func TableColumns(labels []string, width, overhead, minDesc int) (labelWidth, descWidth int) {
	// A labelCap <= 0 must force labelWidth to 0, not fall through to
	// ColumnWidth's own max<=0 "uncapped" convention — that convention
	// exists for callers with no width constraint at all, not for one
	// that computed there's no room left.
	if labelCap := width - overhead - minDesc; labelCap > 0 {
		labelWidth = ColumnWidth(labels, labelCap)
	}

	descWidth = width - overhead - labelWidth
	if descWidth < minDesc {
		descWidth = minDesc
	}
	return labelWidth, descWidth
}
