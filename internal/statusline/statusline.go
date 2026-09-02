// Package statusline renders liam's customizable status block, modeled
// directly on Claude Code's own statusLine primitive: like internal/hook,
// a configured command is run as a child process fed a JSON payload on
// stdin, with no interactive TTY — but it carries none of Hooks' match/
// timeoutMs/async knobs, since a status-block refresh never gates
// anything and always fires standalone (see config.StatusLineConfig). A
// configured command receives session JSON (Input) on stdin; each line it
// prints to stdout becomes one status-block row. An unset command uses
// the built-in default renderer (identity line + metrics bar).
package statusline

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mgoodness/liam/internal/session"
	"github.com/mgoodness/liam/internal/shellrun"
	"github.com/mgoodness/liam/internal/theme"
)

// commandTimeout bounds how long a configured command may run before it's
// killed. It's a var, not a const, so tests can shrink it rather than wait
// out the real value. There's no config knob for this (unlike Hooks'
// TimeoutMs) — a hung statusLine command should never be able to leak a
// goroutine/subprocess indefinitely, regardless of configuration.
var commandTimeout = 5 * time.Second

// maxRows caps how many rows a configured command's output can contribute
// to one refresh — a defensive bound matching ADR-0005's precedent of
// capping external-process output, applied here to row count (a status
// block is logically small) rather than byte size.
const maxRows = 20

// MinRefreshInterval is the floor RefreshInterval enforces on any
// configured positive value.
const MinRefreshInterval = time.Second

// RefreshInterval converts a StatusLineConfig.RefreshInterval millisecond
// value into a time.Duration: ms <= 0 means "no timer configured" (0
// returned); any positive value below MinRefreshInterval is raised to it.
func RefreshInterval(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	d := time.Duration(ms) * time.Millisecond
	if d < MinRefreshInterval {
		return MinRefreshInterval
	}
	return d
}

// Input is the JSON payload a configured statusLine command receives on
// stdin, matching the spec'd stdin schema exactly (no theme or
// permission_mode field — see issue #60's triage correction against
// ADR-0004). ContextWindow is nil until the first response completes, and
// goes back to nil immediately after compaction (session.Session.
// ResetContext), per spec.
type Input struct {
	SessionID     string   `json:"session_id"`
	Cwd           string   `json:"cwd"`
	Model         string   `json:"model"`
	Git           *Git     `json:"git,omitempty"`
	ToolCalls     int      `json:"tool_calls"`
	DurationMs    int64    `json:"duration_ms"`
	CostUSD       float64  `json:"cost_usd"`
	ContextWindow *float64 `json:"context_window"`
}

// Git is Input's "git" field: the working directory's current branch and
// dirty status. Omitted entirely (nil) when cwd isn't inside a git
// repository or the git binary isn't on $PATH.
type Git struct {
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
}

// Tracker accumulates the two statusLine fields session.Session doesn't
// already own — tool-call count and session duration — reset on the same
// session boundaries (New/Clear) as Session's own cost/context tracker.
type Tracker struct {
	started   time.Time
	toolCalls int
}

// NewTracker starts a Tracker with its clock running from now.
func NewTracker() *Tracker {
	return &Tracker{started: time.Now()}
}

// RecordToolCall notes one more completed tool call.
func (t *Tracker) RecordToolCall() {
	t.toolCalls++
}

// Reset restarts the duration clock and zeroes the tool-call count,
// matching /clear's fresh-session semantics.
func (t *Tracker) Reset() {
	t.started = time.Now()
	t.toolCalls = 0
}

// DurationMs reports elapsed time since the Tracker started (or was last
// Reset), in milliseconds.
func (t *Tracker) DurationMs() int64 {
	return time.Since(t.started).Milliseconds()
}

// BuildInput assembles one refresh's Input from sess/tracker's current
// state. model is the fallback shown before any turn has completed
// (sess.LastModel is empty then, e.g. the configured provider.model); once
// a turn completes, the actually-used model (DoneEvent.ModelUsed, threaded
// through Session.Record) takes over, matching the spec's "the
// actually-used model surfaces in the TUI status line." lookup resolves
// ContextWindow's percentage; a nil lookup or a failed resolution leaves
// ContextWindow nil for this refresh (indistinguishable, by design, from
// "no turn completed yet").
func BuildInput(ctx context.Context, sess *session.Session, lookup session.ContextLookup, tracker *Tracker, cwd, model string) Input {
	in := Input{
		SessionID:  sess.ID,
		Cwd:        cwd,
		Model:      model,
		Git:        gitInfo(cwd),
		ToolCalls:  tracker.toolCalls,
		DurationMs: tracker.DurationMs(),
		CostUSD:    sess.Cost(),
	}
	if sess.LastModel != "" {
		in.Model = sess.LastModel
		if lookup != nil {
			if pct, err := sess.ContextPercent(ctx, lookup); err == nil {
				in.ContextWindow = &pct
			}
		}
	}
	return in
}

// gitInfo reports cwd's current branch and dirty status via the git CLI —
// nil if cwd isn't inside a git repository or git isn't on $PATH. Branch
// resolution uses "symbolic-ref" rather than "rev-parse --abbrev-ref",
// since the latter needs at least one commit to resolve HEAD; a fresh repo
// with no commits yet still has a valid current-branch name.
func gitInfo(cwd string) *Git {
	branch, err := runGit(cwd, "symbolic-ref", "--short", "HEAD")
	if err != nil || branch == "" {
		return nil
	}
	dirty := false
	if out, err := runGit(cwd, "status", "--porcelain"); err == nil {
		dirty = out != ""
	}
	return &Git{Branch: branch, Dirty: dirty}
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Render produces one refresh's status-block rows: cmd's stdout lines (one
// row per line) when cmd is non-empty, otherwise the built-in default
// (identity line + metrics bar). Every row is hard-truncated to cols with
// an ellipsis — never wrapped, per spec. cols/lines are passed to a
// configured command's process as COLUMNS/LINES, mirroring what a real
// terminal would set. warn, when non-nil, is called with a one-line
// message if a configured command fails to run or exits non-zero — the
// block is simply empty for that refresh; a failing configured command
// never silently falls back to the built-in renderer, since configuring
// one is an explicit opt-out of the default.
func Render(ctx context.Context, cmd string, in Input, cols, lines int, pal theme.Palette, warn func(string)) []string {
	var rows []string
	if cmd == "" {
		rows = defaultRows(in, pal)
	} else {
		out, err := runCommand(ctx, cmd, in, cols, lines)
		if err != nil {
			if warn != nil {
				warn(fmt.Sprintf("statusLine command %q: %v", cmd, err))
			}
			return nil
		}
		rows = splitLines(out)
	}
	return truncateRows(rows, cols)
}

// runCommand runs command via "sh -c", feeding in as JSON on stdin and
// COLUMNS/LINES as environment variables, and returns its stdout. A
// non-zero exit or a run failure (including commandTimeout's own
// deadline) is an error, with stderr (if any) appended for context.
func runCommand(ctx context.Context, command string, in Input, cols, lines int) (string, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("encoding statusLine input: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	env := []string{fmt.Sprintf("COLUMNS=%d", cols), fmt.Sprintf("LINES=%d", lines)}
	res := shellrun.Run(ctx, command, payload, "", env)
	if res.Err != nil {
		return "", res.Err
	}
	if res.ExitCode != 0 {
		if msg := strings.TrimSpace(res.Stderr); msg != "" {
			return "", fmt.Errorf("exit %d: %s", res.ExitCode, msg)
		}
		return "", fmt.Errorf("exit %d", res.ExitCode)
	}
	return res.Stdout, nil
}

// splitLines turns a command's stdout into one row per line, capped at
// maxRows — a misbehaving command emitting far more output than a status
// block can sensibly use gets truncated with a marker row rather than
// blowing up the layout.
func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	rows := strings.Split(s, "\n")
	if len(rows) > maxRows {
		hidden := len(rows) - maxRows
		rows = append(rows[:maxRows], fmt.Sprintf("… (%d more rows truncated)", hidden))
	}
	return rows
}

func truncateRows(rows []string, width int) []string {
	if width <= 0 {
		return rows
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = ansi.Truncate(r, width, "…")
	}
	return out
}

// defaultRows renders the built-in default: an identity line (badge, model,
// cwd, git branch/dirty) and a metrics bar (context %, tool-call count,
// elapsed duration, running cost), matching the prototype's design
// (prototype/tui-shell's statusBlock) with one deliberate departure: the
// prototype's mockup ended the metrics line with the active theme's name,
// but ticket #27's resolution (which superseded the prototype on this
// point) dropped theme entirely — "not needed in the status line
// itself" — so it's replaced here with the now-real running cost the
// prototype could only mock.
func defaultRows(in Input, pal theme.Palette) []string {
	badge := lipgloss.NewStyle().
		Foreground(lipgloss.Color(pal.Base)).
		Background(lipgloss.Color(pal.Mauve)).
		Bold(true).Padding(0, 1).Render("Liam")
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Subtext))

	var segs []string
	if in.Model != "" {
		segs = append(segs, in.Model)
	}
	if in.Cwd != "" {
		segs = append(segs, in.Cwd)
	}
	if in.Git != nil {
		branch := "🌿 " + in.Git.Branch
		if in.Git.Dirty {
			branch += " *"
		}
		segs = append(segs, branch)
	}
	line1 := badge + " " + dim.Render(strings.Join(segs, "  •  "))

	return []string{line1, metricsBar(in, pal)}
}

func metricsBar(in Input, pal theme.Palette) string {
	pct := 0.0
	ctxLabel := "n/a ctx"
	if in.ContextWindow != nil {
		pct = *in.ContextWindow
		ctxLabel = fmt.Sprintf("%d%% ctx", int(pct*100))
	}

	filled := int(pct * 10)
	if filled > 10 {
		filled = 10
	}
	if filled < 0 {
		filled = 0
	}
	barColor := pal.Green
	switch {
	case pct >= 0.9:
		barColor = pal.Red
	case pct >= 0.7:
		barColor = pal.Yellow
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(barColor)).Render(strings.Repeat("▓", filled)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Overlay)).Render(strings.Repeat("░", 10-filled))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Subtext))

	toolWord := "tools"
	if in.ToolCalls == 1 {
		toolWord = "tool"
	}
	elapsed := time.Duration(in.DurationMs) * time.Millisecond
	return bar + dim.Render(fmt.Sprintf(" %s  •  %d %s run  •  ⏱ %s  •  $%.4f",
		ctxLabel, in.ToolCalls, toolWord, FormatDuration(elapsed), in.CostUSD))
}

// FormatDuration renders d to the nearest second as "Ns" or "Mm0Ss" —
// coarser than a hundredths-precision stopwatch, matching the granularity
// a status readout actually needs. Shared by the status block's own
// session-cumulative elapsed time and the TUI's per-turn indicator (issue
// #144, tui.formatElapsed's original home before it moved here), so the
// two elapsed-time readouts in the TUI read consistently rather than each
// hand-rolling the same rounding.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
