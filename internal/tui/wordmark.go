package tui

// This file renders the startup banner's "LIAM" wordmark (issue #169) as a
// small hand-drawn dot-matrix block face, styled after charmbracelet/
// crush's own wordmark technique (internal/ui/logo — see that package's
// letterforms.go): each letter is a fixed-size grid of "on" cells rendered
// as "█", stitched together into rows.
//
// Two earlier attempts are visible in this file's git history and were
// dropped after visual review: a block-art rendering decoded from
// assets/liam.png (the spec's first choice, at two different resolutions)
// came out either illegible or noisy, and a per-column color gradient
// across this same letterform data (Crush's own approach) broke each
// letter's visual continuity into disconnected color patches at this small
// a scale. A single flat brand color reads clearly at both, so that's what
// this settles on.

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// wordmarkColor is liam's fixed brand color (sampled from assets/liam.png),
// used for the banner's wordmark regardless of the active theme.Palette —
// the one deliberate exception to every other themed color in this
// package, per the spec's "not remapped to the active Catppuccin theme
// palette".
const wordmarkColor = "#7D6249"

// wordmarkGap is the blank column count separating adjacent letters, and
// wordmarkRows is every letterform's fixed row count — both letters (see
// wordmarkLetters) and renderWordmark rely on this being the same for
// every letter so the rows line up.
const (
	wordmarkGap  = 1
	wordmarkRows = 5
)

// wordmarkLetters spells "LIAM" (CONTEXT.md's Banner name, capitalized to
// suit the block face) as one entry per letter, each already-padded to that
// letter's own fixed width across all wordmarkRows rows.
var wordmarkLetters = [][]string{
	{ // L
		"█ ",
		"█ ",
		"█ ",
		"█ ",
		"██",
	},
	{ // I
		"█",
		"█",
		"█",
		"█",
		"█",
	},
	{ // A
		" █ ",
		"█ █",
		"███",
		"█ █",
		"█ █",
	},
	{ // M
		"█   █",
		"██ ██",
		"█ █ █",
		"█   █",
		"█   █",
	},
}

// renderWordmark renders wordmarkLetters as a wordmarkRows-line block face,
// every "on" cell styled in the fixed wordmarkColor.
func renderWordmark() string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(wordmarkColor)).Bold(true)
	gap := strings.Repeat(" ", wordmarkGap)

	rows := make([]string, wordmarkRows)
	for r := 0; r < wordmarkRows; r++ {
		letterRows := make([]string, len(wordmarkLetters))
		for i, letter := range wordmarkLetters {
			letterRows[i] = letter[r]
		}
		rows[r] = style.Render(strings.Join(letterRows, gap))
	}
	return strings.Join(rows, "\n")
}
