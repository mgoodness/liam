package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/theme"
)

// inputBorderTopHeight and inputBorderBottomHeight are the one-row-each
// top and bottom border lines renderInputBox draws around the input
// textarea's own content (no left/right sides) — folded into View()'s
// inputRow (top only, since that's what shifts where the input's own text
// starts) and syncViewportDims' reserved budget (both, the input box's
// full on-screen footprint), the same way popupDialogHeight and
// indicatorHeight already are.
const (
	inputBorderTopHeight    = 1
	inputBorderBottomHeight = 1
	inputBorderHeight       = inputBorderTopHeight + inputBorderBottomHeight
)

// renderInputBox wraps the input textarea's already-rendered content in a
// top-and-bottom-only border (no left/right sides) — the input area's
// visual separator from the transcript above and the status block below.
// Unlike renderPopupDialog's fixed popupDialogHeight, this doesn't pin a
// fixed height: the border simply frames however many rows content
// already has, so the textarea's own DynamicHeight growth (1 to
// ta.MaxHeight rows) passes through unconstrained.
func renderInputBox(p theme.Palette, width int, content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(lipgloss.Color(p.Overlay)).
		Background(lipgloss.Color(p.Base)).
		Width(width).
		Render(content)
}
