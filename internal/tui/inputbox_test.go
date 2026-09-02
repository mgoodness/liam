package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
)

// TestRenderInputBoxHeightGrowsWithContentPlusBorder covers the contrast
// with renderPopupDialog's fixed popupDialogHeight: the input box has no
// pinned height of its own, so its rendered height must track whatever
// content it's given (the textarea's own DynamicHeight growth) plus
// exactly the two border rows, for both a single line and several.
func TestRenderInputBoxHeightGrowsWithContentPlusBorder(t *testing.T) {
	p := New(agent.Loop{}, config.Config{}, nil).pal
	width := 40

	for _, n := range []int{1, 3, 8} {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = "input text"
		}
		content := strings.Join(lines, "\n")

		box := renderInputBox(p, width, content)
		if got, want := lipgloss.Height(box), n+inputBorderHeight; got != want {
			t.Errorf("n=%d: renderInputBox height = %d, want %d (content + inputBorderHeight)", n, got, want)
		}
		if got := lipgloss.Width(box); got != width {
			t.Errorf("n=%d: renderInputBox width = %d, want %d", n, got, width)
		}
	}
}

// TestRenderInputBoxHasNoLeftOrRightBorder covers the AC that the input
// box's border is top-and-bottom only: the content row must start flush
// at column 0 (no leading vertical border rune eating into it), unlike
// renderPopupDialog's full RoundedBorder on every side.
func TestRenderInputBoxHasNoLeftOrRightBorder(t *testing.T) {
	p := New(agent.Loop{}, config.Config{}, nil).pal
	box := renderInputBox(p, 20, "hello")

	rows := strings.Split(box, "\n")
	if len(rows) != 1+inputBorderHeight {
		t.Fatalf("renderInputBox rows = %d, want %d (top border, 1 content row, bottom border)", len(rows), 1+inputBorderHeight)
	}
	content := ansi.Strip(rows[inputBorderTopHeight])
	if !strings.HasPrefix(content, "hello") {
		t.Errorf("content row = %q, want it to start with %q (no left border rune)", content, "hello")
	}
}
