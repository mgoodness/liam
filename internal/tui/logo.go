package tui

//go:generate go run gen_logo.go

// This file holds the startup banner's block-art logo (issue #169):
// logoTop/logoBottom (logo_gen.go) are generated from the checked-in
// assets/liam.png by gen_logo.go, decoding it into a fixed grid of
// half-block terminal cells classified into liam's two fixed brand colors
// (logoColorDark/logoColorLight) or transparent — never remapped to the
// active theme.Palette, per the spec.

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// logoColorDark and logoColorLight are liam's fixed brand colors, sampled
// directly from assets/liam.png (see gen_logo.go). They render the same
// regardless of theme.mode — the one deliberate exception to every other
// themed color in this package.
const (
	logoColorDark  = "#251910"
	logoColorLight = "#7D6249"
)

// logoPixel is one half-block cell's classification, shared by both
// logoTop and logoBottom.
type logoPixel byte

const (
	logoTransparent logoPixel = ' ' // renders as nothing — the surrounding background shows through
	logoDark        logoPixel = 'D'
	logoLight       logoPixel = 'L'
)

// renderLogo renders logoTop/logoBottom as a multi-line block-art string,
// one "▀" (upper-half-block) rune per cell: its foreground is the top
// pixel's color, its background the bottom pixel's — the standard trick for
// packing two vertically-stacked pixels into one monospace character cell.
// A cell transparent on both halves renders as a plain space rather than a
// styled glyph, letting the transcript's own background style (see
// renderTranscript) show through instead of a hardcoded color.
func renderLogo() string {
	rows := make([]string, len(logoTop))
	for i, top := range logoTop {
		bottom := logoBottom[i]
		var b strings.Builder
		for j := 0; j < len(top); j++ {
			b.WriteString(renderLogoCell(logoPixel(top[j]), logoPixel(bottom[j])))
		}
		rows[i] = b.String()
	}
	return strings.Join(rows, "\n")
}

// renderLogoCell renders one half-block cell from its top/bottom
// classification. A glyph's unstyled foreground/background would render as
// the terminal's (or lipgloss's inherited) default color, not as
// "transparent" — so a half that's logoTransparent is never carried as a
// style at all: both transparent is a plain space, one side transparent
// picks the single-half glyph ("▀" or "▄") colored only by the opaque side,
// and both opaque uses "▀" with the top as foreground and bottom as
// background, the standard two-pixels-per-cell packing.
func renderLogoCell(top, bottom logoPixel) string {
	topHex, topOK := logoHex(top)
	bottomHex, bottomOK := logoHex(bottom)

	switch {
	case !topOK && !bottomOK:
		return " "
	case topOK && !bottomOK:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(topHex)).Render("▀")
	case !topOK && bottomOK:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(bottomHex)).Render("▄")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(topHex)).Background(lipgloss.Color(bottomHex)).Render("▀")
	}
}

// logoHex maps a logoPixel to its fixed brand-color hex string; ok is false
// for logoTransparent, meaning "leave this half unstyled".
func logoHex(p logoPixel) (string, bool) {
	switch p {
	case logoDark:
		return logoColorDark, true
	case logoLight:
		return logoColorLight, true
	default:
		return "", false
	}
}
