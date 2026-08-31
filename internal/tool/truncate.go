package tool

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// outputCap is the max bytes a tool result keeps before truncate cuts it,
// matching web_fetch's own spec'd convention (ADR-0005).
const outputCap = 8000

// truncate caps s at outputCap bytes for a Result.Content value. s at or
// under the cap is returned unchanged. Over the cap, it cuts at the last
// newline at or before outputCap (never mid-line) and appends a marker
// noting how many bytes were cut; a single line longer than outputCap with
// no newline to cut at falls back to a hard cut at outputCap. Either way,
// the cut is then backed up to the nearest UTF-8 rune boundary at or before
// it, so a multi-byte rune straddling the cut point is dropped whole rather
// than split into invalid UTF-8.
func truncate(s string) string {
	if len(s) <= outputCap {
		return s
	}

	cut := outputCap
	if i := strings.LastIndexByte(s[:outputCap], '\n'); i >= 0 {
		cut = i + 1
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n[... truncated, %d more bytes ...]", len(s)-cut)
}
