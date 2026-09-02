package tui

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/theme"
)

// clampColumn clamps col to a valid index into line — textarea.Column() can
// point one past the end of a shorter line than the one it was measured
// against (e.g. after Value() and Line()/Column() are read across separate
// calls).
func clampColumn(line []rune, col int) int {
	if col > len(line) {
		return len(line)
	}
	return col
}

// findTokenStart scans line backward from col (exclusive) for an unbroken
// "<trigger>query" token with no whitespace between trigger and col, the
// backward-scan shape both the "@" and "/" popups' update steps share.
// spacePreceded reports whether a trigger preceded by whitespace (not just
// the start of the line) counts as a token start: true for "@" (issue #58),
// false for "/", which is only ever a command candidate at column 0 (issue
// #137). Returns ok=false when col isn't inside such a token.
func findTokenStart(line []rune, col int, trigger rune, spacePreceded bool) (int, bool) {
	for i := col - 1; i >= 0; i-- {
		switch {
		case line[i] == trigger:
			if i == 0 || spacePreceded && unicode.IsSpace(line[i-1]) {
				return i, true
			}
			return 0, false
		case unicode.IsSpace(line[i]):
			return 0, false
		}
	}
	return 0, false
}

// popupSelectedIndex recomputes a popup's selected index for an incoming
// recompute: it carries prevSelected across only when the popup was already
// active (same token still under the cursor, guaranteed by the caller) and
// the previous index is still in range of the new match list — so typing
// more of a query doesn't reset the user's highlight unless it's no longer
// on the list. Returns 0 (the top of the list) when there's nothing to
// carry over.
func popupSelectedIndex(active bool, prevSelected, newLen int) int {
	if active && prevSelected < newLen {
		return prevSelected
	}
	return 0
}

// moveCursorToOffset places ta's cursor at the given 0-indexed rune offset
// into value, converting the flat offset into textarea's (row, col) terms.
// Used after SetValue, which always leaves the cursor at the very end of
// the new value.
func moveCursorToOffset(ta *textarea.Model, value string, offset int) {
	lines := strings.Split(value, "\n")
	row, col := len(lines)-1, len([]rune(lines[len(lines)-1]))
	remaining := offset
	for i, l := range lines {
		n := len([]rune(l))
		if remaining <= n {
			row, col = i, remaining
			break
		}
		remaining -= n + 1
	}

	for r := len(lines) - 1; r > row; r-- {
		ta.CursorUp()
	}
	ta.SetCursorColumn(col)
}

// popupDialogHeight is the fixed total on-screen height (border rows
// included) a floating popup dialog (issue #139) occupies whenever one is
// active, regardless of its current match count — no per-keystroke
// resizing as results narrow or widen.
//
// lipgloss.Style.Height(N), with a Border set, renders exactly N rows
// total: it pads a shorter block up to N but does not grow a taller one
// past it (verified empirically — a bordered block already at or past N
// content+border rows overflows N instead of being cropped to it). So N
// must itself already leave room for the 2 border rows on top of however
// many content rows the dialog needs, hence maxMentionMatches + 2 rather
// than maxMentionMatches alone: maxMentionMatches (== maxSlashMatches) is
// the cap on match-list rows, so a full list's 8 content rows plus a
// 2-row border need N = 10 to render at a constant height; passing 8
// (issue #139's stated round number, written before this border-overhead
// arithmetic was worked out) would let a fuller list overflow past a
// shorter one's rendered size — the opposite of "no per-keystroke
// resizing".
const popupDialogHeight = maxMentionMatches + 2

// popupBorderWidth is how many of a popup dialog's width columns
// renderPopupDialog's RoundedBorder eats — 1 each side, no padding — so a
// content renderer (renderSlashPopup) can size its own columns against
// what's actually left over.
const popupBorderWidth = 2

// renderPopupDialog frames content — renderMentionPopup's or
// renderSlashPopup's already-styled match list — as a bordered floating
// dialog (issue #139's "floating bordered dialog", replacing the previous
// appended-row rendering), exactly width columns by popupDialogHeight rows
// regardless of how many lines content has: lipgloss.Style.Height pads a
// shorter block with blank interior rows rather than shrinking the border
// around it, which is what keeps the dialog's footprint constant across
// keystrokes.
func renderPopupDialog(p theme.Palette, width int, content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(p.Overlay)).
		Background(lipgloss.Color(p.Surface)).
		Foreground(lipgloss.Color(p.Text)).
		Width(width).
		Height(popupDialogHeight).
		Render(content)
}

// popupActive reports whether either autocomplete popup (m.mention or
// m.slash) is currently open. The two are mutually exclusive in practice —
// handleKey's doc comment notes a token can only start with one leading
// character at a time — so at most one of them ever contributes a dialog.
func (m Model) popupActive() bool {
	return m.mention.active || m.slash.active
}

// activePopupContent returns the active popup's already-styled match-list
// content, or "" when neither m.mention nor m.slash is active.
func (m Model) activePopupContent() string {
	switch {
	case m.mention.active:
		return renderMentionPopup(m.pal, m.mention)
	case m.slash.active:
		return renderSlashPopup(m.pal, m.slash, m.width)
	}
	return ""
}
