package hook

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mgoodness/liam/internal/config"
)

// captureWarnings collects every Warn message under a mutex, safe for the
// async-hook tests that fire warnings from a goroutine.
type captureWarnings struct {
	mu   sync.Mutex
	msgs []string
}

func (c *captureWarnings) fn() func(string) {
	return func(msg string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.msgs = append(c.msgs, msg)
	}
}

func (c *captureWarnings) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.msgs...)
}

func TestBeforeToolAllowsWhenNoHooksConfigured(t *testing.T) {
	r := &Runner{}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked", d)
	}
}

func TestBeforeToolAllowsOnZeroExit(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{{Command: "exit 0"}},
	}}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked", d)
	}
}

func TestBeforeToolBlocksOnNonZeroExitAndSurfacesStderr(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{{Command: `echo "no shell commands allowed" >&2; exit 1`}},
	}}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if !d.Blocked {
		t.Fatalf("Decision = %+v, want blocked", d)
	}
	if d.Reason != "no shell commands allowed" {
		t.Errorf("Reason = %q, want %q", d.Reason, "no shell commands allowed")
	}
}

func TestBeforeToolFallsBackToExitCodeWhenStderrEmpty(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{{Command: "exit 3"}},
	}}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if !d.Blocked {
		t.Fatalf("Decision = %+v, want blocked", d)
	}
	if !strings.Contains(d.Reason, "exit 3") {
		t.Errorf("Reason = %q, want a mention of the exit code", d.Reason)
	}
}

func TestBeforeToolMatchRestrictsToNamedTools(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{{Command: "exit 1", Match: []string{"edit"}}},
	}}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (bash doesn't match [edit])", d)
	}
}

func TestBeforeToolWildcardMatchesEveryTool(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{{Command: "exit 1", Match: []string{"*"}}},
	}}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if !d.Blocked {
		t.Errorf("Decision = %+v, want blocked (\"*\" matches every tool)", d)
	}
}

func TestBeforeToolStopsAtFirstBlockingHook(t *testing.T) {
	dir := t.TempDir()
	secondRanPath := dir + "/second-ran"
	r := &Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{
			{Command: "exit 1"},
			{Command: "touch " + secondRanPath}, // must never run
		},
	}}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if !d.Blocked {
		t.Fatalf("Decision = %+v, want blocked by the first hook", d)
	}
	if fileExists(secondRanPath) {
		t.Error("second hook ran after the first already blocked")
	}
}

// TestBeforeToolAsyncHookNeverBlocks covers async: true's fire-and-forget
// contract: an async hook can't gate the call it's attached to, even when
// it would otherwise deny (non-zero exit).
func TestBeforeToolAsyncHookNeverBlocks(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	r := &Runner{Hooks: config.HooksConfig{
		BeforeTool: []config.HookConfig{{Command: "exit 1", Async: true}},
	}}
	r.Warn = func(string) { wg.Done() }

	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (async hook)", d)
	}

	waitOrTimeout(t, &wg)
}

// TestBeforeToolFailsOpenOnCommandNotFound covers ADR-0002: a hook whose
// command can't even be started fails open (allow) with a logged warning,
// rather than blocking.
func TestBeforeToolFailsOpenOnCommandNotFound(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{BeforeTool: []config.HookConfig{{Command: "/no/such/binary-liam-test"}}},
		Warn:  cw.fn(),
	}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (fail open)", d)
	}
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestBeforeToolFailsOpenOnTimeout covers ADR-0002's other fail-open case: a
// hook that doesn't return within TimeoutMs fails open rather than blocking.
func TestBeforeToolFailsOpenOnTimeout(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{BeforeTool: []config.HookConfig{{Command: "sleep 5; exit 1", TimeoutMs: 50}}},
		Warn:  cw.fn(),
	}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (timeout fails open)", d)
	}
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestBeforeToolFailsOpenOnSignalKill covers ADR-0002's "crashes before
// exiting" fail-open case: a hook process killed by a signal (here, the
// shell sending itself SIGKILL) reports ExitCode() == -1 per os/exec's
// documented behavior, which must fail open rather than being treated as a
// deny verdict.
func TestBeforeToolFailsOpenOnSignalKill(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{BeforeTool: []config.HookConfig{{Command: "kill -9 $$"}}},
		Warn:  cw.fn(),
	}
	d := r.BeforeTool(context.Background(), "bash", `{}`)
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (signal kill fails open)", d)
	}
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestRunReceivesStdinJSONAndLIAMEnvVars drives a real "sh -c" hook that
// writes its stdin JSON (line 1) and the LIAM_* env vars it saw (remaining
// lines) to a temp file, then reads the file back to assert on both.
func TestRunReceivesStdinJSONAndLIAMEnvVars(t *testing.T) {
	dir := t.TempDir()
	outPath := dir + "/hook-saw.json"
	cmd := `{ cat; echo; } > ` + outPath + `; { echo "LIFECYCLE=$LIAM_LIFECYCLE"; echo "SESSION=$LIAM_SESSION_ID"; echo "CWD=$LIAM_CWD"; echo "TOOL=$LIAM_TOOL_NAME"; echo "DISABLED=$LIAM_HOOKS_DISABLED"; } >> ` + outPath

	r := &Runner{
		Hooks:     config.HooksConfig{BeforeTool: []config.HookConfig{{Command: cmd}}},
		SessionID: "sess-123",
		Cwd:       dir,
	}
	d := r.BeforeTool(context.Background(), "bash", `{"command":"ls"}`)
	if d.Blocked {
		t.Fatalf("Decision = %+v, want not blocked", d)
	}

	got := readFile(t, outPath)

	var stdin struct {
		Lifecycle string `json:"lifecycle"`
		SessionID string `json:"sessionId"`
		Cwd       string `json:"cwd"`
		Tool      struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		} `json:"tool"`
	}
	firstLine := strings.SplitN(got, "\n", 2)[0]
	if err := json.Unmarshal([]byte(firstLine), &stdin); err != nil {
		t.Fatalf("unmarshaling captured stdin JSON %q: %v", firstLine, err)
	}
	if stdin.Lifecycle != "beforeTool" {
		t.Errorf("stdin lifecycle = %q, want beforeTool", stdin.Lifecycle)
	}
	if stdin.SessionID != "sess-123" {
		t.Errorf("stdin sessionId = %q, want sess-123", stdin.SessionID)
	}
	if stdin.Tool.Name != "bash" {
		t.Errorf("stdin tool.name = %q, want bash", stdin.Tool.Name)
	}
	if string(stdin.Tool.Args) != `{"command":"ls"}` {
		t.Errorf("stdin tool.args = %s, want %s", stdin.Tool.Args, `{"command":"ls"}`)
	}

	if !strings.Contains(got, "LIFECYCLE=beforeTool") {
		t.Errorf("captured env = %q, want LIAM_LIFECYCLE=beforeTool", got)
	}
	if !strings.Contains(got, "SESSION=sess-123") {
		t.Errorf("captured env = %q, want LIAM_SESSION_ID=sess-123", got)
	}
	if !strings.Contains(got, "TOOL=bash") {
		t.Errorf("captured env = %q, want LIAM_TOOL_NAME=bash", got)
	}
	if !strings.Contains(got, "DISABLED=1") {
		t.Errorf("captured env = %q, want LIAM_HOOKS_DISABLED=1", got)
	}
}

func TestSessionStartAndSessionEndRunConfiguredHooks(t *testing.T) {
	dir := t.TempDir()
	startPath := dir + "/started"
	endPath := dir + "/ended"
	r := &Runner{Hooks: config.HooksConfig{
		SessionStart: []config.HookConfig{{Command: "touch " + startPath}},
		SessionEnd:   []config.HookConfig{{Command: "touch " + endPath}},
	}}

	r.SessionStart(context.Background())
	r.SessionEnd(context.Background())

	if !fileExists(startPath) {
		t.Error("sessionStart hook did not run")
	}
	if !fileExists(endPath) {
		t.Error("sessionEnd hook did not run")
	}
}

func TestAfterToolNeverBlocksOnNonZeroExit(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: "exit 1"}}},
		Warn:  cw.fn(),
	}
	// AfterTool has no return value to assert "not blocked" on; the
	// contract under test is simply that it runs to completion (doesn't
	// panic or hang) and reports the failure via Warn.
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	if len(cw.all()) == 0 {
		t.Error("Warn was never called for a failing afterTool hook")
	}
}

func TestAfterToolReceivesResult(t *testing.T) {
	dir := t.TempDir()
	outPath := dir + "/result"
	r := &Runner{Hooks: config.HooksConfig{
		AfterTool: []config.HookConfig{{Command: "cat > " + outPath}},
	}}
	r.AfterTool(context.Background(), "bash", `{}`, "some output", true)

	var stdin struct {
		Result struct {
			Content string `json:"content"`
			IsError bool   `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(readFile(t, outPath)), &stdin); err != nil {
		t.Fatalf("unmarshaling captured stdin: %v", err)
	}
	if stdin.Result.Content != "some output" || !stdin.Result.IsError {
		t.Errorf("stdin.result = %+v, want {some output true}", stdin.Result)
	}
}

func waitOrTimeout(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the async hook's Warn callback")
	}
}
