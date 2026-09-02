package render

import (
	"strings"
	"testing"
)

func TestTruncateWidth(t *testing.T) {
	cases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"fits exactly", "hello", 5, "hello"},
		{"shorter than n", "hi", 5, "hi"},
		{"overflow gets ellipsis", "hello world", 5, "hell…"},
		{"n zero", "hello", 0, ""},
		{"n negative", "hello", -1, ""},
		{"multibyte runes counted not bytes", "héllo world", 5, "héll…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateWidth(tc.s, tc.n); got != tc.want {
				t.Errorf("TruncateWidth(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
		})
	}
}

func TestColumnWidthLongestLabel(t *testing.T) {
	got := ColumnWidth([]string{"a", "bbb", "cc"}, 0)
	if got != 3 {
		t.Errorf("ColumnWidth(...) = %d, want 3 (longest label, uncapped)", got)
	}
}

func TestColumnWidthCapsAtMax(t *testing.T) {
	got := ColumnWidth([]string{"a", "verylonglabel"}, 5)
	if got != 5 {
		t.Errorf("ColumnWidth(...) = %d, want 5 (capped)", got)
	}
}

func TestColumnWidthZeroOrNegativeMaxIsUncapped(t *testing.T) {
	got := ColumnWidth([]string{"verylonglabel"}, 0)
	if got != len("verylonglabel") {
		t.Errorf("ColumnWidth(..., 0) = %d, want %d (uncapped)", got, len("verylonglabel"))
	}
}

func TestTableColumnsFitsWithinBudget(t *testing.T) {
	labels := []string{"a", "much-longer-label"}
	width, overhead := 40, 5

	labelWidth, descWidth := TableColumns(labels, width, overhead, MinDescWidth)
	if got := overhead + labelWidth + descWidth; got != width {
		t.Errorf("overhead(%d) + labelWidth(%d) + descWidth(%d) = %d, want exactly width %d", overhead, labelWidth, descWidth, got, width)
	}
	if labelWidth != len("much-longer-label") {
		t.Errorf("labelWidth = %d, want %d (the longest label, uncapped — plenty of room)", labelWidth, len("much-longer-label"))
	}
}

func TestTableColumnsCapsLabelToProtectMinDescWidth(t *testing.T) {
	// A label long enough that giving it full width would starve the
	// description column below minDesc — labelWidth must give ground
	// instead.
	labels := []string{strings.Repeat("x", 100)}
	width, overhead := 40, 5

	labelWidth, descWidth := TableColumns(labels, width, overhead, MinDescWidth)
	if descWidth != MinDescWidth {
		t.Errorf("descWidth = %d, want exactly MinDescWidth (%d) — the label must be capped to protect it", descWidth, MinDescWidth)
	}
	if want := width - overhead - MinDescWidth; labelWidth != want {
		t.Errorf("labelWidth = %d, want %d", labelWidth, want)
	}
}

func TestTableColumnsNarrowWidthStillFloorsDescAtMinDesc(t *testing.T) {
	// A width so narrow even labelWidth=0 can't leave room for minDesc:
	// descWidth still comes back exactly minDesc rather than going
	// negative, and labelWidth is capped to 0, not left uncapped.
	labels := []string{"averagename"}
	labelWidth, descWidth := TableColumns(labels, 5, 5, MinDescWidth)

	if labelWidth != 0 {
		t.Errorf("labelWidth = %d, want 0 (capped, not the uncapped fallback ColumnWidth's max<=0 would otherwise give)", labelWidth)
	}
	if descWidth != MinDescWidth {
		t.Errorf("descWidth = %d, want MinDescWidth (%d)", descWidth, MinDescWidth)
	}
}

func TestFitLabelDescLeavesShortLabelUntouched(t *testing.T) {
	label, desc := FitLabelDesc("a", "a short description", 10, 20, 80)
	if label != "a" {
		t.Errorf("label = %q, want %q (fits, no truncation)", label, "a")
	}
	if desc != "a short description" {
		t.Errorf("desc = %q, want unchanged", desc)
	}
}

func TestFitLabelDescTruncatesOverflowingLabel(t *testing.T) {
	label, _ := FitLabelDesc(strings.Repeat("x", 20), "d", 10, 20, 80)
	if got := len([]rune(label)); got != 10 {
		t.Errorf("len(label) = %d, want 10 (truncated to labelWidth)", got)
	}
}

func TestFitLabelDescTruncatesDescUnconditionally(t *testing.T) {
	_, desc := FitLabelDesc("a", strings.Repeat("x", 50), 10, 20, 80)
	if got := len([]rune(desc)); got != 20 {
		t.Errorf("len(desc) = %d, want 20 (truncated to descWidth)", got)
	}
}

func TestFitLabelDescNoWidthSkipsTruncation(t *testing.T) {
	longLabel, longDesc := strings.Repeat("x", 100), strings.Repeat("y", 100)
	label, desc := FitLabelDesc(longLabel, longDesc, 10, 20, 0)
	if label != longLabel || desc != longDesc {
		t.Errorf("FitLabelDesc(..., width=0) truncated, want both left untouched")
	}
}
