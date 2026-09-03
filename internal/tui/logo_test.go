package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestRenderLogoFixedWidthAndHeight covers the generated block art's shape:
// every row is the same rune width (a rectangular block, not a ragged
// silhouette that would misalign against the text beside it), and the row
// count matches logoTop/logoBottom.
func TestRenderLogoFixedWidthAndHeight(t *testing.T) {
	got := renderLogo()
	rows := strings.Split(got, "\n")
	if len(rows) != len(logoTop) {
		t.Fatalf("renderLogo() has %d rows, want %d (len(logoTop))", len(rows), len(logoTop))
	}
	want := lipgloss.Width(rows[0])
	for i, row := range rows {
		if w := lipgloss.Width(row); w != want {
			t.Errorf("renderLogo() row %d width = %d, want %d (every row the same width)", i, w, want)
		}
	}
}

// TestRenderLogoColorsAreFixedBrandHex covers the acceptance criterion that
// the logo's colors never change with the active theme — logoColorDark/
// logoColorLight are baked-in hex constants, so the rendered art must
// literally carry those escape sequences regardless of any theme.Palette.
func TestRenderLogoColorsAreFixedBrandHex(t *testing.T) {
	got := renderLogo()
	dark := lipgloss.NewStyle().Foreground(lipgloss.Color(logoColorDark))
	light := lipgloss.NewStyle().Foreground(lipgloss.Color(logoColorLight))
	if !strings.Contains(got, dark.Render("▀")) && !strings.Contains(got, dark.Render("▄")) {
		t.Error("renderLogo() doesn't contain the fixed dark brand color")
	}
	if !strings.Contains(got, light.Render("▀")) && !strings.Contains(got, light.Render("▄")) {
		t.Error("renderLogo() doesn't contain the fixed light brand color")
	}
}
