package tui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/theme"
)

// maxMentionMatches caps how many find results the "@" popup shows at once
// — a small on-screen list, unrelated to tool.MaxSearchResults' much larger
// model-facing cap.
const maxMentionMatches = 8

// mentionState tracks an in-progress "@"-file-reference autocomplete
// (issue #58): active while the cursor sits inside an unbroken "@query"
// token, closing automatically the moment that's no longer true.
type mentionState struct {
	active   bool
	line     int // row (textarea.Line()) the mention was opened on
	start    int // rune column of the '@' on that row
	query    string
	matches  []string
	selected int
}

// fileRefRangeRe splits a mention query's optional trailing ":42" or
// ":10-20" line-range suffix from the file part it filters on.
var fileRefRangeRe = regexp.MustCompile(`^(.*):(\d+)(?:-(\d+))?$`)

// parseFileReference splits query into its file-search part and an optional
// 1-indexed inclusive line range. hasRange is false (and start/end are 0)
// when query has no ":N"/":N-M" suffix, or the suffix doesn't parse into a
// sane non-empty ascending range.
func parseFileReference(query string) (filePart string, start, end int, hasRange bool) {
	m := fileRefRangeRe.FindStringSubmatch(query)
	if m == nil {
		return query, 0, 0, false
	}

	start, err := strconv.Atoi(m[2])
	if err != nil || start < 1 {
		return query, 0, 0, false
	}
	end = start
	if m[3] != "" {
		end, err = strconv.Atoi(m[3])
		if err != nil || end < start {
			return query, 0, 0, false
		}
	}
	return m[1], start, end, true
}

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

// findMentionStart scans line backward from col (exclusive) for an unbroken
// "@" token: an '@' preceded by whitespace or the start of the line, with no
// whitespace between it and col. Returns ok=false when col isn't inside such
// a token.
func findMentionStart(line []rune, col int) (int, bool) {
	for i := col - 1; i >= 0; i-- {
		switch {
		case line[i] == '@':
			if i == 0 || unicode.IsSpace(line[i-1]) {
				return i, true
			}
			return 0, false
		case unicode.IsSpace(line[i]):
			return 0, false
		}
	}
	return 0, false
}

// updateMention recomputes m.mention from the textarea's current cursor
// position, closing the popup when the cursor no longer sits inside an
// unbroken "@query" token or no findSearcher was attached (WithFindSearcher
// unset). Preserves the selected index across a recompute that still
// targets the same token, so typing more of the query doesn't reset the
// user's highlighted match unless it's no longer in range.
func (m *Model) updateMention() {
	if m.findSearcher == nil {
		m.mention = mentionState{}
		return
	}

	lines := strings.Split(m.input.Value(), "\n")
	row := m.input.Line()
	if row >= len(lines) {
		m.mention = mentionState{}
		return
	}
	lineRunes := []rune(lines[row])
	col := clampColumn(lineRunes, m.input.Column())

	start, ok := findMentionStart(lineRunes, col)
	if !ok {
		m.mention = mentionState{}
		return
	}

	query := string(lineRunes[start+1 : col])
	filePart, _, _, _ := parseFileReference(query)

	matches, _, err := m.findSearcher.Find(context.Background(), filePart)
	if err != nil {
		matches = nil
	}
	if len(matches) > maxMentionMatches {
		matches = matches[:maxMentionMatches]
	}

	selected := 0
	if m.mention.active && m.mention.line == row && m.mention.start == start && m.mention.selected < len(matches) {
		selected = m.mention.selected
	}

	m.mention = mentionState{
		active:   true,
		line:     row,
		start:    start,
		query:    query,
		matches:  matches,
		selected: selected,
	}
}

// selectMention replaces the active "@query" token with the highlighted
// match's content, inlined and delimited per issue #58's spec, then closes
// the popup. A no-op (menu just closes) when there are no matches to pick
// from.
func (m Model) selectMention() Model {
	if len(m.mention.matches) == 0 {
		m.mention = mentionState{}
		return m
	}
	path := m.mention.matches[m.mention.selected]
	_, start, end, hasRange := parseFileReference(m.mention.query)

	inserted, err := renderFileReference(path, start, end, hasRange)
	if err != nil {
		inserted = fmt.Sprintf("[file: %s — error: %v]", path, err)
	}

	lines := strings.Split(m.input.Value(), "\n")
	lineRunes := []rune(lines[m.mention.line])
	col := clampColumn(lineRunes, m.input.Column())

	before := string(lineRunes[:m.mention.start])
	after := string(lineRunes[col:])

	var prefixOffset int
	for _, l := range lines[:m.mention.line] {
		prefixOffset += len([]rune(l)) + 1
	}
	offset := prefixOffset + len([]rune(before)) + len([]rune(inserted))

	lines[m.mention.line] = before + inserted + after
	newValue := strings.Join(lines, "\n")

	m.input.SetValue(newValue)
	moveCursorToOffset(&m.input, newValue, offset)
	m.mention = mentionState{}
	return m
}

// renderFileReference reads path and formats it (or just lines start-end,
// 1-indexed inclusive, with "Line N: " context, when hasRange) delimited by
// a "[file: ...]"/"[/file: ...]" marker pair so the model can distinguish
// inlined content from the user's own text.
func renderFileReference(path string, start, end int, hasRange bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if !hasRange {
		return fmt.Sprintf("[file: %s]\n%s\n[/file: %s]", path, string(data), path), nil
	}

	lines := strings.Split(string(data), "\n")
	if start > len(lines) {
		return "", fmt.Errorf("%s: line range %d-%d out of bounds (file has %d lines)", path, start, end, len(lines))
	}
	if end > len(lines) {
		end = len(lines)
	}

	label := fmt.Sprintf("%s:%d", path, start)
	if end != start {
		label = fmt.Sprintf("%s:%d-%d", path, start, end)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[file: %s]\n", label)
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "Line %d: %s\n", i, lines[i-1])
	}
	fmt.Fprintf(&b, "[/file: %s]", label)
	return b.String(), nil
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

// renderMentionPopup renders the "@"-autocomplete match list shown below
// the input while ms is active.
func renderMentionPopup(p theme.Palette, ms mentionState) string {
	if len(ms.matches) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Italic(true).Render("  (no matching files)")
	}

	var b strings.Builder
	for i, path := range ms.matches {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i == ms.selected {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(p.Base)).Background(lipgloss.Color(p.Blue)).Bold(true).Render("› " + path))
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Render("  " + path))
	}
	return b.String()
}
