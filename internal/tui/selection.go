package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// point is a (line, column) position in the transcript's absolute
// content-line space (viewport.YOffset() + screen Y, screen X at the time
// of the event) rather than screen-relative, so a selection's endpoints
// stay anchored to the same transcript text even as later events are
// processed against a possibly-different YOffset.
type point struct{ line, col int }

// selectionState tracks an in-progress or just-completed click-drag text
// selection over the transcript (issue #142). Mouse-wheel scrolling (issue
// #59) already puts the transcript into tea.MouseModeCellMotion, which
// routes every mouse event to liam instead of letting the terminal do
// native click-drag selection — see
// docs/adr/0006-osc52-transcript-selection.md for why this is built as real
// in-app selection (copying via OSC-52 on release) rather than a keybinding
// to toggle mouse mode off.
//
// The zero value (start == end) means "nothing selected" — both a fresh
// Model and a completed no-drag click land here, so hasRange doubles as
// both "draw a highlight" and "there's something to copy".
type selectionState struct {
	dragging   bool // true from mouse-down through mouse-up; false once released (the highlight itself outlives this)
	start, end point
}

// hasRange reports whether sel spans more than a single point. A plain
// click with no movement between press and release must neither highlight
// nor copy anything (issue #142's AC) — encoding "nothing selected" as
// start == end, rather than a separate bool, means the zero value is
// already correct.
func (sel selectionState) hasRange() bool {
	return sel.start != sel.end
}

// normalized returns sel's span in reading order (top-to-bottom,
// left-to-right) regardless of which direction the drag actually went, so a
// drag up-and-left highlights/copies identically to the same span dragged
// down-and-right.
func (sel selectionState) normalized() (lo, hi point) {
	if sel.start.line < sel.end.line || (sel.start.line == sel.end.line && sel.start.col <= sel.end.col) {
		return sel.start, sel.end
	}
	return sel.end, sel.start
}

// rowBounds returns the column span within row i that falls inside the
// [lo, hi] selection: the row's full width for any line strictly between
// the endpoints, clipped to lo.col on the first selected line and hi.col on
// the last (which may be the same line, clipping both ends of a
// single-row selection). rowWidth is the display width of row i's own
// content — selectedText and highlightSelection each measure a different
// underlying line slice, so it's the caller's to supply.
func rowBounds(i int, lo, hi point, rowWidth int) (start, end int) {
	start, end = 0, rowWidth
	if i == lo.line {
		start = lo.col
	}
	if i == hi.line {
		end = hi.col
	}
	return start, end
}

// transcriptRow reports whether screen row y falls within the transcript's
// own on-screen region (the viewport's rows), as opposed to the popup,
// input, or status block below it — the scoping the AC requires: a mouse
// interaction starting outside the transcript must never engage selection.
func (m Model) transcriptRow(y int) bool {
	return y >= 0 && y < m.viewport.Height()
}

// handleMouseClick starts a new selection when the press lands in the
// transcript's own screen region with the left button; any other click
// (right/middle button, or landing in the popup/input/status block) leaves
// m.sel cleared and does nothing else — mouse messages are handled
// exclusively here rather than forwarded to the textarea, so there's no
// fallback path to preserve. Starting a new selection unconditionally clears
// any prior one first, matching ordinary terminal-app click-to-deselect
// behavior.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	m.sel = selectionState{}
	if mouse.Button != tea.MouseLeft || !m.transcriptRow(mouse.Y) {
		return m, nil
	}
	m.syncViewportDims()
	p := point{line: m.viewport.YOffset() + mouse.Y, col: mouse.X}
	m.sel = selectionState{dragging: true, start: p, end: p}
	m.refreshViewport()
	return m, nil
}

// handleMouseMotion extends the in-progress selection while the left button
// is held (m.sel.dragging); a motion event with no drag active — a hover, or
// one that started outside the transcript — is a no-op, matching cell-motion
// tracking's own behavior of only reporting motion during a button press.
// Coordinates are clamped to the viewport's rows/columns rather than
// ignored once they leave that region, so dragging past the transcript's
// edge extends the selection to that edge instead of stalling.
func (m Model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if !m.sel.dragging {
		return m, nil
	}
	mouse := msg.Mouse()
	y := clampInt(mouse.Y, 0, max(0, m.viewport.Height()-1))
	x := clampInt(mouse.X, 0, max(0, m.width-1))
	m.sel.end = point{line: m.viewport.YOffset() + y, col: x}
	m.refreshViewport()
	return m, nil
}

// handleMouseRelease ends an in-progress selection: a genuine drag (the
// release point differs from the press point) copies the spanned plain text
// to the system clipboard via OSC-52 (tea.SetClipboard) with no confirmation
// step, and leaves the highlight in place. A release with no movement (a
// plain click) clears the selection entirely and copies nothing, per the AC.
func (m Model) handleMouseRelease(_ tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if !m.sel.dragging {
		return m, nil
	}
	m.sel.dragging = false
	if !m.sel.hasRange() {
		m.sel = selectionState{}
		m.refreshViewport()
		return m, nil
	}
	text := m.selectedText()
	m.refreshViewport()
	if text == "" {
		return m, nil
	}
	return m, tea.SetClipboard(text)
}

// selectedText extracts the plain (ANSI-stripped) text spanned by m.sel from
// the current transcript render, one rendered row per selected line joined
// by "\n" — the AC's "line breaks preserved as rendered" for a selection
// spanning multiple wrapped/rendered lines. Each row is right-trimmed since
// renderTranscript pads every row to the full viewport width with
// background-colored spaces that aren't meaningful selected content.
func (m Model) selectedText() string {
	if !m.sel.hasRange() {
		return ""
	}
	lines := strings.Split(m.renderTranscript(), "\n")
	lo, hi := m.sel.normalized()
	lo.line = clampInt(lo.line, 0, max(0, len(lines)-1))
	hi.line = clampInt(hi.line, 0, max(0, len(lines)-1))

	rows := make([]string, 0, hi.line-lo.line+1)
	for i := lo.line; i <= hi.line; i++ {
		start, end := rowBounds(i, lo, hi, ansi.StringWidth(lines[i]))
		rows = append(rows, strings.TrimRight(ansi.Strip(ansi.Cut(lines[i], start, end)), " "))
	}
	return strings.Join(rows, "\n")
}

// highlightSelection returns a copy of lines (the already palette-styled,
// width-padded rows renderTranscript produces) with m.sel's span rendered in
// reverse video — the AC's "highlights it with reverse video as the drag
// proceeds". It uses lipgloss.StyleRanges, the same column-range styling
// mechanism bubbles/viewport itself uses for its own search-match
// highlighting, so the highlight composes with whatever role-based color
// each line already carries outside the selected span.
func highlightSelection(lines []string, sel selectionState) []string {
	if !sel.hasRange() {
		return lines
	}
	lo, hi := sel.normalized()
	lo.line = clampInt(lo.line, 0, max(0, len(lines)-1))
	hi.line = clampInt(hi.line, 0, max(0, len(lines)-1))

	out := make([]string, len(lines))
	copy(out, lines)
	style := lipgloss.NewStyle().Reverse(true)
	for i := lo.line; i <= hi.line; i++ {
		start, end := rowBounds(i, lo, hi, ansi.StringWidth(out[i]))
		if end <= start {
			continue
		}
		out[i] = lipgloss.StyleRanges(out[i], lipgloss.NewRange(start, end, style))
	}
	return out
}

// clampInt confines v to [lo, hi] inclusive.
func clampInt(v, lo, hi int) int {
	return min(hi, max(lo, v))
}
