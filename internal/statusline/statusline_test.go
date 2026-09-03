package statusline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/session"
	"github.com/mgoodness/liam/internal/theme"
)

func TestRefreshIntervalFloorsAndDisables(t *testing.T) {
	cases := []struct {
		ms   int
		want int // milliseconds
	}{
		{ms: 0, want: 0},
		{ms: -5, want: 0},
		{ms: 500, want: 1000},
		{ms: 1000, want: 1000},
		{ms: 2500, want: 2500},
	}
	for _, tc := range cases {
		if got := RefreshInterval(tc.ms).Milliseconds(); got != int64(tc.want) {
			t.Errorf("RefreshInterval(%d) = %dms, want %dms", tc.ms, got, tc.want)
		}
	}
}

func TestTrackerRecordsAndResets(t *testing.T) {
	tr := NewTracker()
	tr.RecordToolCall()
	tr.RecordToolCall()

	in := BuildInput(context.Background(), session.New(), nil, tr, "/tmp", "openrouter/auto")
	if in.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", in.ToolCalls)
	}

	tr.Reset()
	in = BuildInput(context.Background(), session.New(), nil, tr, "/tmp", "openrouter/auto")
	if in.ToolCalls != 0 {
		t.Errorf("ToolCalls after Reset = %d, want 0", in.ToolCalls)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{9 * time.Second, "9s"},
		{65 * time.Second, "1m05s"},
		{125 * time.Second, "2m05s"},
	}
	for _, tc := range cases {
		if got := FormatDuration(tc.d); got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestBuildInputBeforeAnyTurnFallsBackToConfiguredModelAndNilContext(t *testing.T) {
	sess := session.New()
	in := BuildInput(context.Background(), sess, nil, NewTracker(), "/proj", "openrouter/auto")

	if in.Model != "openrouter/auto" {
		t.Errorf("Model = %q, want the configured fallback before any turn", in.Model)
	}
	if in.ContextWindow != nil {
		t.Errorf("ContextWindow = %v, want nil before any turn", in.ContextWindow)
	}
	if in.SessionID != sess.ID {
		t.Errorf("SessionID = %q, want %q", in.SessionID, sess.ID)
	}
}

type fakeLookup struct {
	max int
	err error
}

func (f fakeLookup) MaxContextLength(context.Context, string) (int, error) {
	return f.max, f.err
}

func TestBuildInputAfterTurnUsesActualModelAndResolvesContextWindow(t *testing.T) {
	sess := session.New()
	sess.Record("anthropic/claude-3.7-sonnet", provider.Usage{InputTokens: 5000, CachedInputTokens: 5000, CostUSD: 0.02})

	in := BuildInput(context.Background(), sess, fakeLookup{max: 100000}, NewTracker(), "/proj", "openrouter/auto")

	if in.Model != "anthropic/claude-3.7-sonnet" {
		t.Errorf("Model = %q, want the actually-used model", in.Model)
	}
	if in.ContextWindow == nil {
		t.Fatal("ContextWindow = nil, want the resolved percentage")
	}
	if got, want := *in.ContextWindow, 0.1; got != want {
		t.Errorf("ContextWindow = %v, want %v", got, want)
	}
	if in.CostUSD != 0.02 {
		t.Errorf("CostUSD = %v, want 0.02", in.CostUSD)
	}
}

func TestBuildInputContextWindowNilOnLookupError(t *testing.T) {
	sess := session.New()
	sess.Record("anthropic/claude-3.7-sonnet", provider.Usage{InputTokens: 100})

	in := BuildInput(context.Background(), sess, fakeLookup{err: errBoom}, NewTracker(), "/proj", "openrouter/auto")
	if in.ContextWindow != nil {
		t.Errorf("ContextWindow = %v, want nil on a lookup error", in.ContextWindow)
	}
}

var errBoom = errors.New("boom")

func TestGitInfoOutsideRepoIsNil(t *testing.T) {
	if got := gitInfo(t.TempDir()); got != nil {
		t.Errorf("gitInfo() = %+v, want nil outside a git repository", got)
	}
}

func TestGitInfoInsideRepoReportsBranchAndDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on $PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q", "-b", "test-branch")

	got := gitInfo(dir)
	if got == nil {
		t.Fatal("gitInfo() = nil, want a Git result inside a repo")
	}
	if got.Branch != "test-branch" {
		t.Errorf("Branch = %q, want %q", got.Branch, "test-branch")
	}
	if got.Dirty {
		t.Error("Dirty = true, want false in a freshly initialized repo")
	}

	if err := os.WriteFile(dir+"/untracked.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = gitInfo(dir)
	if got == nil || !got.Dirty {
		t.Errorf("Dirty = %v, want true with an untracked file present", got)
	}
}

func TestRenderDefaultProducesIdentityAndMetricsRows(t *testing.T) {
	rows := Render(context.Background(), "", Input{Model: "openrouter/auto", Cwd: "/proj"}, 200, 50, theme.Frappe, nil)
	if len(rows) != 2 {
		t.Fatalf("Render() = %d rows, want 2 (identity + metrics)", len(rows))
	}
	if !strings.Contains(rows[0], "openrouter/auto") || !strings.Contains(rows[0], "/proj") {
		t.Errorf("identity row = %q, want it to mention the model and cwd", rows[0])
	}
	if !strings.Contains(rows[1], "n/a ctx") {
		t.Errorf("metrics row = %q, want n/a ctx before any turn", rows[1])
	}
}

// TestRenderDefaultCollapsesCwdToHome covers display-only "~" collapsing
// (render.CollapseHome): the identity row shows "~/project", never the raw
// home-directory path.
func TestRenderDefaultCollapsesCwdToHome(t *testing.T) {
	t.Setenv("HOME", "/home/liam")

	rows := Render(context.Background(), "", Input{Model: "openrouter/auto", Cwd: "/home/liam/project"}, 200, 50, theme.Frappe, nil)
	if !strings.Contains(rows[0], "~/project") {
		t.Errorf("identity row = %q, want it to contain %q", rows[0], "~/project")
	}
	if strings.Contains(rows[0], "/home/liam/project") {
		t.Errorf("identity row = %q, want the raw home-directory path collapsed away", rows[0])
	}
}

func TestRenderCommandReceivesInputAsStdinJSON(t *testing.T) {
	in := Input{SessionID: "abc", Cwd: "/proj", Model: "openrouter/auto", ToolCalls: 3, CostUSD: 0.5}
	want, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	rows := Render(context.Background(), "cat", in, 200, 50, theme.Frappe, nil)
	if len(rows) != 1 || rows[0] != string(want) {
		t.Errorf("Render() = %q, want [%q]", rows, want)
	}
}

func TestRenderCommandGetsColumnsAndLinesEnv(t *testing.T) {
	rows := Render(context.Background(), `echo "$COLUMNS,$LINES"`, Input{}, 123, 45, theme.Frappe, nil)
	if len(rows) != 1 || rows[0] != "123,45" {
		t.Errorf("Render() = %q, want [\"123,45\"]", rows)
	}
}

func TestRenderCommandMultipleLinesBecomeMultipleRows(t *testing.T) {
	rows := Render(context.Background(), `printf 'one\ntwo\nthree\n'`, Input{}, 200, 50, theme.Frappe, nil)
	want := []string{"one", "two", "three"}
	if len(rows) != len(want) {
		t.Fatalf("Render() = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("rows[%d] = %q, want %q", i, rows[i], want[i])
		}
	}
}

func TestRenderCommandFailureWarnsAndReturnsNoRows(t *testing.T) {
	var warned string
	rows := Render(context.Background(), `echo "boom" >&2; exit 1`, Input{}, 200, 50, theme.Frappe, func(msg string) { warned = msg })
	if rows != nil {
		t.Errorf("Render() = %v, want nil rows on a failing command", rows)
	}
	if warned == "" {
		t.Error("warn was never called for a failing statusLine command")
	}
	if !strings.Contains(warned, "boom") {
		t.Errorf("warn message = %q, want it to include the command's stderr", warned)
	}
}

// TestRenderCommandTimesOutRatherThanHangingForever covers the
// no-config-knob safety bound: a command that never exits must still be
// killed, warned about, and never leak the refresh forever.
func TestRenderCommandTimesOutRatherThanHangingForever(t *testing.T) {
	old := commandTimeout
	commandTimeout = 50 * time.Millisecond
	defer func() { commandTimeout = old }()

	var warned string
	rows := Render(context.Background(), "sleep 5", Input{}, 200, 50, theme.Frappe, func(msg string) { warned = msg })
	if rows != nil {
		t.Errorf("Render() = %v, want nil rows for a timed-out command", rows)
	}
	if warned == "" {
		t.Error("warn was never called for a timed-out command")
	}
}

// TestRenderCommandCapsRowCount covers the defensive maxRows bound: a
// misbehaving command emitting far more lines than a status block can use
// gets truncated with a marker row rather than blowing up the layout.
func TestRenderCommandCapsRowCount(t *testing.T) {
	var script strings.Builder
	for i := range maxRows + 5 {
		fmt.Fprintf(&script, "echo line%d; ", i)
	}
	rows := Render(context.Background(), script.String(), Input{}, 200, 50, theme.Frappe, nil)

	if len(rows) != maxRows+1 {
		t.Fatalf("len(rows) = %d, want %d (maxRows + 1 marker row)", len(rows), maxRows+1)
	}
	if rows[0] != "line0" {
		t.Errorf("rows[0] = %q, want %q", rows[0], "line0")
	}
	if !strings.Contains(rows[maxRows], "5 more rows truncated") {
		t.Errorf("last row = %q, want a truncation marker mentioning 5 more rows", rows[maxRows])
	}
}

func TestRenderTruncatesWideLinesWithEllipsisNotWrap(t *testing.T) {
	rows := Render(context.Background(), `echo 0123456789abcdef`, Input{}, 10, 50, theme.Frappe, nil)
	if len(rows) != 1 {
		t.Fatalf("Render() = %v, want exactly 1 row (no wrapping)", rows)
	}
	if !strings.HasSuffix(rows[0], "…") {
		t.Errorf("row = %q, want a hard-truncated ellipsis", rows[0])
	}
	if len([]rune(rows[0])) != 10 {
		t.Errorf("row width = %d runes, want 10 (truncated to cols)", len([]rune(rows[0])))
	}
}
