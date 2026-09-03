package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestRenderWordmarkFixedRowCount covers the block face's shape: exactly
// wordmarkRows lines, matching every letterform's own row count.
func TestRenderWordmarkFixedRowCount(t *testing.T) {
	got := strings.Split(renderWordmark(), "\n")
	if len(got) != wordmarkRows {
		t.Errorf("renderWordmark() has %d rows, want %d (wordmarkRows)", len(got), wordmarkRows)
	}
}

// TestRenderWordmarkFixedColor covers the acceptance criterion that the
// wordmark's color never changes with the active theme — wordmarkColor is a
// baked-in hex constant, so the rendered face must literally carry that
// escape sequence.
func TestRenderWordmarkFixedColor(t *testing.T) {
	got := strings.Split(renderWordmark(), "\n")
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(wordmarkColor)).Bold(true)
	want := style.Render(strings.Join(letterRow(0), " "))
	if got[0] != want {
		t.Errorf("renderWordmark() row 0 = %q, want %q (styled in the fixed brand color)", got[0], want)
	}
}

// letterRow reports every letter's row r, in wordmarkLetters order — a
// small test helper mirroring renderWordmark's own row-assembly loop.
func letterRow(r int) []string {
	rows := make([]string, len(wordmarkLetters))
	for i, letter := range wordmarkLetters {
		rows[i] = letter[r]
	}
	return rows
}

// TestWordmarkLettersConsistentRowsPerLetter covers renderWordmark's own
// assumption: every letter in wordmarkLetters has exactly wordmarkRows
// rows, and every row within a given letter is the same rune width — a
// ragged letter would misalign every row after it.
func TestWordmarkLettersConsistentRowsPerLetter(t *testing.T) {
	for i, letter := range wordmarkLetters {
		if len(letter) != wordmarkRows {
			t.Errorf("wordmarkLetters[%d] has %d rows, want %d (wordmarkRows)", i, len(letter), wordmarkRows)
		}
		want := len([]rune(letter[0]))
		for r, row := range letter {
			if w := len([]rune(row)); w != want {
				t.Errorf("wordmarkLetters[%d] row %d width = %d, want %d (every row in a letter the same width)", i, r, w, want)
			}
		}
	}
}
