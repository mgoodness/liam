package tui

// This file implements the animated "turn in progress" indicator (issue
// #144), styled after Crush's (charmbracelet/crush, internal/ui/anim)
// "thinking" spinner: a fixed-width run of glyphs that continuously
// cycle/scramble, colored by blending across the active theme.Palette's
// gradient rather than a single static color, with newly-revealed cells
// fading in from the background rather than popping in all at once.
// Alongside the glyphs it shows the current turn's live elapsed time, the
// name of whatever tool is currently executing (if any), and a running
// token-usage estimate.
//
// It renders in its own region directly above the input — see tui.go's
// View()/syncViewportDims, which fold indicatorHeight in exactly the way
// they already fold in popupDialogHeight — for as long as Model.busy is
// true, and disappears the instant busy goes false (turn completion or
// Escape-driven interruption), matching the spec's reuse of busy as the
// single show/hide gate rather than a parallel flag.
//
// Model's own turnStart/activeTool/turnOutputChars/animFrame/indicatorTick
// fields, and the Update/submit/compact integration points that drive
// them, stay in tui.go/compact.go, matching mention.go/statusline.go's own
// split between "shared Model state" and "this feature's own file".

import (
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/statusline"
	"github.com/mgoodness/liam/internal/theme"
)

// indicatorHeight is the indicator's fixed on-screen row count, folded
// into syncViewportDims'/View()'s layout math the same way popupDialogHeight
// is: a single row, always present in full the instant it's shown (no
// per-frame resizing).
const indicatorHeight = 1

// defaultIndicatorTick is the animation's real-time cadence in production —
// fast enough to read as continuously alive, slow enough not to burn CPU
// repainting. It's a var, not a const, so tests can shrink it to 0 and
// avoid real sleeps when driving the indicator's tick loop through drain(),
// matching statusline.commandTimeout's own shrinkable-var precedent.
var defaultIndicatorTick = 90 * time.Millisecond

// charsPerTokenEstimate is the lightweight character-count heuristic behind
// the indicator's live, in-progress output-token estimate. There's no
// tokenizer anywhere in the codebase, and the spec is explicit that an
// approximation — one that moves and lands close, not an exact count — is
// sufficient here; only the post-turn figure (DoneEvent's authoritative
// Usage) needs to be exact.
const charsPerTokenEstimate = 4

// indicatorGlyphs is the charset each of the indicator's fixed cells
// scrambles through — dense braille block glyphs, chosen for a busy,
// textured look distinct from a single rotating spinner frame.
var indicatorGlyphs = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏⣾⣽⣻⢿⡿⣟⣯⣷")

// indicatorWidth is the fixed number of glyph cells the animation renders,
// independent of terminal width — per spec, "a fixed-width run of glyphs".
const indicatorWidth = 14

// indicatorRevealStagger/indicatorRevealFrames stage each cell's fade-in:
// cell i starts revealing indicatorRevealStagger*i frames after the
// indicator first appears (turnStart/animFrame reset to 0, see
// startIndicator), ramping linearly to full opacity over
// indicatorRevealFrames — so the glyph run visibly builds left-to-right
// rather than every cell popping in at once.
const (
	indicatorRevealStagger = 1
	indicatorRevealFrames  = 6
)

// indicatorTickMsg advances the indicator's animation frame. It's only
// rescheduled for as long as Model.busy stays true (see Update's handler in
// tui.go); the instant a turn ends, the next indicatorTickMsg to arrive
// finds busy false and simply stops the loop rather than rescheduling
// again, so no ticks (and no goroutines) leak past a turn's end.
type indicatorTickMsg struct{}

// indicatorTickCmd schedules the next indicatorTickMsg, m.indicatorTick
// from now. Ticking runs as an ordinary tea.Cmd on Bubbletea's async
// command runner, so it never blocks Update's handling of any other
// message — key presses (including Escape-driven interruption) are
// processed independently the moment they arrive.
func (m Model) indicatorTickCmd() tea.Cmd {
	return tea.Tick(m.indicatorTick, func(time.Time) tea.Msg { return indicatorTickMsg{} })
}

// startIndicator resets the per-turn state a freshly-started turn (submit)
// or compaction (compact) begins with: turnStart backs the live
// elapsed-time readout (a new per-turn clock, distinct from
// statusline.Tracker's session-cumulative DurationMs), and the rest give
// the animation and token estimate a clean slate rather than carrying over
// whatever the previous turn left behind.
func (m *Model) startIndicator() {
	m.turnStart = time.Now()
	m.animFrame = 0
	m.activeTool = ""
	m.turnOutputChars = 0
}

// stopIndicator clears the per-turn fields a finished turn (finishTurn) or
// compaction (the compactDoneMsg handler) leaves behind, so a stale tool
// name or token count can't flash back into view if busy is ever
// momentarily true again before the next submit() resets them properly.
func (m *Model) stopIndicator() {
	m.activeTool = ""
	m.turnOutputChars = 0
}

// estimatedOutputTokens converts chars — text accumulated from
// TextDeltaEvents since the last DoneEvent — into a rough output-token
// count via charsPerTokenEstimate. Deliberately approximate: see the
// package doc on DoneEvent's authoritative Usage superseding it the moment
// a turn (or a tool-call round trip within one) completes.
func estimatedOutputTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return chars / charsPerTokenEstimate
}

// renderIndicator renders the current animation frame: elapsed time is
// time.Since(m.turnStart), and the live token figure is the session's
// running total across every already-completed turn plus this turn's own
// in-progress estimate — the "baseline plus estimate" layering the spec
// calls for.
func (m Model) renderIndicator() string {
	elapsed := time.Since(m.turnStart)
	tokens := m.sess.TotalOutputTokens + estimatedOutputTokens(m.turnOutputChars)
	return renderTurnIndicator(m.pal, m.animFrame, elapsed, m.activeTool, tokens)
}

// renderTurnIndicator renders one frame of the indicator: the scrambling,
// gradient-colored glyph run, then elapsed time, the active tool's name
// (when a tool call is in flight — omitted between calls, during plain
// text generation), and the live token estimate.
func renderTurnIndicator(pal theme.Palette, frame int, elapsed time.Duration, toolName string, tokens int) string {
	glyphs := renderIndicatorGlyphs(pal, frame)

	label := statusline.FormatDuration(elapsed)
	if toolName != "" {
		label += " · " + toolName
	}
	label += fmt.Sprintf(" · ~%d tokens", tokens)

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Subtext))
	return glyphs + "  " + dim.Render(label)
}

// renderIndicatorGlyphs renders indicatorWidth cells, each independently
// cycling through indicatorGlyphs on frame's cadence (glyphIndex staggers
// each cell's own rate so they don't all change in lockstep, the
// "scramble" look) and colored by sampling a gradient built from the
// active palette's accent colors via lipgloss.Blend1D — the gradient's
// sample point shifts by frame per cell too, sweeping the color band
// across the run rather than holding it static. A cell not yet past its
// staggered reveal offset (see indicatorRevealStagger/Frames) is blended
// toward the palette's own background instead of shown at full color, the
// fade-in.
func renderIndicatorGlyphs(pal theme.Palette, frame int) string {
	gradient := lipgloss.Blend1D(indicatorWidth,
		lipgloss.Color(pal.Blue), lipgloss.Color(pal.Green), lipgloss.Color(pal.Yellow),
		lipgloss.Color(pal.Red), lipgloss.Color(pal.Mauve), lipgloss.Color(pal.Blue),
	)
	bg := lipgloss.Color(pal.Base)

	var b strings.Builder
	for i := range indicatorWidth {
		g := indicatorGlyphs[glyphIndex(frame, i)]
		col := gradient[(i+frame)%len(gradient)]
		col = fadeBlend(bg, col, revealProgress(frame, i))
		b.WriteString(lipgloss.NewStyle().Foreground(col).Render(string(g)))
	}
	return b.String()
}

// glyphIndex picks cell i's glyph at frame: cells advance through
// indicatorGlyphs at slightly different strides (1+cell%3) with a
// per-cell phase offset (cell*7), so neighboring cells rarely land on the
// same glyph at the same frame.
func glyphIndex(frame, cell int) int {
	n := len(indicatorGlyphs)
	return (frame*(1+cell%3) + cell*7) % n
}

// revealProgress reports cell i's fade-in progress at frame, in [0,1]: 0
// before its staggered start (indicatorRevealStagger*i), ramping linearly
// to 1 over indicatorRevealFrames, then holding at 1 — the steady-state
// scramble/gradient sweep continues indefinitely once a cell is fully
// revealed.
func revealProgress(frame, cell int) float64 {
	start := cell * indicatorRevealStagger
	if frame <= start {
		return 0
	}
	p := float64(frame-start) / float64(indicatorRevealFrames)
	if p > 1 {
		p = 1
	}
	return p
}

// fadeBlend blends fg toward bg by way of lipgloss.Blend1D's CIELAB
// interpolation: t=0 is bg (invisible against the transcript background),
// t=1 is fg at full color, matching Crush's own staggered fade-in feel
// rather than each new glyph popping straight to full opacity.
func fadeBlend(bg, fg color.Color, t float64) color.Color {
	if t >= 1 {
		return fg
	}
	if t <= 0 {
		return bg
	}
	const steps = 21
	blended := lipgloss.Blend1D(steps, bg, fg)
	idx := int(t * float64(steps-1))
	return blended[idx]
}

// countChars reports s's rune count, the "characters" charsPerTokenEstimate
// is denominated in — utf8.RuneCountInString rather than len(s), so
// multi-byte UTF-8 text doesn't inflate the estimate.
func countChars(s string) int {
	return utf8.RuneCountInString(s)
}
