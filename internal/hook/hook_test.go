package hook

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
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

func TestUserPromptSubmitAllowsWhenNoHooksConfigured(t *testing.T) {
	r := &Runner{}
	d := r.UserPromptSubmit(context.Background(), "hello")
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked", d)
	}
}

func TestUserPromptSubmitAllowsOnZeroExit(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: "exit 0"}},
	}}
	d := r.UserPromptSubmit(context.Background(), "hello")
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked", d)
	}
}

func TestUserPromptSubmitBlocksOnNonZeroExitAndSurfacesStderr(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: `echo "no prompts about foo" >&2; exit 1`}},
	}}
	d := r.UserPromptSubmit(context.Background(), "tell me about foo")
	if !d.Blocked {
		t.Fatalf("Decision = %+v, want blocked", d)
	}
	if d.Reason != "no prompts about foo" {
		t.Errorf("Reason = %q, want %q", d.Reason, "no prompts about foo")
	}
}

func TestUserPromptSubmitFallsBackToExitCodeWhenStderrEmpty(t *testing.T) {
	r := &Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: "exit 3"}},
	}}
	d := r.UserPromptSubmit(context.Background(), "hello")
	if !d.Blocked {
		t.Fatalf("Decision = %+v, want blocked", d)
	}
	if !strings.Contains(d.Reason, "exit 3") {
		t.Errorf("Reason = %q, want a mention of the exit code", d.Reason)
	}
}

func TestUserPromptSubmitStopsAtFirstBlockingHook(t *testing.T) {
	dir := t.TempDir()
	secondRanPath := dir + "/second-ran"
	r := &Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{
			{Command: "exit 1"},
			{Command: "touch " + secondRanPath}, // must never run
		},
	}}
	d := r.UserPromptSubmit(context.Background(), "hello")
	if !d.Blocked {
		t.Fatalf("Decision = %+v, want blocked by the first hook", d)
	}
	if fileExists(secondRanPath) {
		t.Error("second hook ran after the first already blocked")
	}
}

// TestUserPromptSubmitAsyncHookNeverBlocks covers async: true's
// fire-and-forget contract: an async hook can't gate the submission it's
// attached to, even when it would otherwise deny (non-zero exit).
func TestUserPromptSubmitAsyncHookNeverBlocks(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	r := &Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: "exit 1", Async: true}},
	}}
	r.Warn = func(string) { wg.Done() }

	d := r.UserPromptSubmit(context.Background(), "hello")
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (async hook)", d)
	}

	waitOrTimeout(t, &wg)
}

// TestUserPromptSubmitFailsOpenOnCommandNotFound covers ADR-0002: a hook
// whose command can't even be started fails open (allow) with a logged
// warning, rather than blocking the prompt.
func TestUserPromptSubmitFailsOpenOnCommandNotFound(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{UserPromptSubmit: []config.HookConfig{{Command: "/no/such/binary-liam-test"}}},
		Warn:  cw.fn(),
	}
	d := r.UserPromptSubmit(context.Background(), "hello")
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (fail open)", d)
	}
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestUserPromptSubmitFailsOpenOnTimeout covers ADR-0002's other fail-open
// case: a hook that doesn't return within TimeoutMs fails open rather than
// blocking the prompt.
func TestUserPromptSubmitFailsOpenOnTimeout(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{UserPromptSubmit: []config.HookConfig{{Command: "sleep 5; exit 1", TimeoutMs: 50}}},
		Warn:  cw.fn(),
	}
	d := r.UserPromptSubmit(context.Background(), "hello")
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (timeout fails open)", d)
	}
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestUserPromptSubmitFailsOpenOnSignalKill covers ADR-0002's "crashes
// before exiting" fail-open case for the new blocking lifecycle point, same
// as TestBeforeToolFailsOpenOnSignalKill.
func TestUserPromptSubmitFailsOpenOnSignalKill(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{UserPromptSubmit: []config.HookConfig{{Command: "kill -9 $$"}}},
		Warn:  cw.fn(),
	}
	d := r.UserPromptSubmit(context.Background(), "hello")
	if d.Blocked {
		t.Errorf("Decision = %+v, want not blocked (signal kill fails open)", d)
	}
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestUserPromptSubmitReceivesRawText asserts the hook's stdin JSON carries
// the exact text passed in, under "prompt.text".
func TestUserPromptSubmitReceivesRawText(t *testing.T) {
	dir := t.TempDir()
	outPath := dir + "/prompt-saw.json"
	r := &Runner{Hooks: config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: "cat > " + outPath}},
	}}
	d := r.UserPromptSubmit(context.Background(), "/foo bar baz")
	if d.Blocked {
		t.Fatalf("Decision = %+v, want not blocked", d)
	}

	var stdin struct {
		Lifecycle string `json:"lifecycle"`
		Prompt    struct {
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(readFile(t, outPath)), &stdin); err != nil {
		t.Fatalf("unmarshaling captured stdin: %v", err)
	}
	if stdin.Lifecycle != "userPromptSubmit" {
		t.Errorf("stdin lifecycle = %q, want userPromptSubmit", stdin.Lifecycle)
	}
	if stdin.Prompt.Text != "/foo bar baz" {
		t.Errorf("stdin.prompt.text = %q, want %q", stdin.Prompt.Text, "/foo bar baz")
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

func TestAgentDoneNeverBlocksOnNonZeroExit(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AgentDone: []config.HookConfig{{Command: "exit 1"}}},
		Warn:  cw.fn(),
	}
	// AgentDone has no return value to assert "not blocked" on; the
	// contract under test is simply that it runs to completion (doesn't
	// panic or hang) and reports the failure via Warn.
	r.AgentDone(context.Background(), "stop", "openrouter/auto", provider.Usage{InputTokens: 1})
	if len(cw.all()) == 0 {
		t.Error("Warn was never called for a failing agentDone hook")
	}
}

// TestAgentDoneReceivesPayload asserts the hook's stdin JSON mirrors
// DoneEvent's fields under "done".
func TestAgentDoneReceivesPayload(t *testing.T) {
	dir := t.TempDir()
	outPath := dir + "/done-saw.json"
	r := &Runner{Hooks: config.HooksConfig{
		AgentDone: []config.HookConfig{{Command: "cat > " + outPath}},
	}}
	r.AgentDone(context.Background(), "stop", "openrouter/auto", provider.Usage{InputTokens: 10, OutputTokens: 20, CachedInputTokens: 3, CostUSD: 0.05})

	var stdin struct {
		Lifecycle string `json:"lifecycle"`
		Done      struct {
			FinishReason string         `json:"finishReason"`
			ModelUsed    string         `json:"modelUsed"`
			Usage        provider.Usage `json:"usage"`
		} `json:"done"`
	}
	if err := json.Unmarshal([]byte(readFile(t, outPath)), &stdin); err != nil {
		t.Fatalf("unmarshaling captured stdin: %v", err)
	}
	if stdin.Lifecycle != "agentDone" {
		t.Errorf("stdin lifecycle = %q, want agentDone", stdin.Lifecycle)
	}
	want := struct {
		FinishReason string
		ModelUsed    string
		Usage        provider.Usage
	}{"stop", "openrouter/auto", provider.Usage{InputTokens: 10, OutputTokens: 20, CachedInputTokens: 3, CostUSD: 0.05}}
	if stdin.Done.FinishReason != want.FinishReason || stdin.Done.ModelUsed != want.ModelUsed || stdin.Done.Usage != want.Usage {
		t.Errorf("stdin.done = %+v, want %+v", stdin.Done, want)
	}
}

// TestAgentDoneFailsOpenOnCommandNotFound covers ADR-0002 for the new
// observer lifecycle point, same fail-open contract as the existing 4.
func TestAgentDoneFailsOpenOnCommandNotFound(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AgentDone: []config.HookConfig{{Command: "/no/such/binary-liam-test"}}},
		Warn:  cw.fn(),
	}
	r.AgentDone(context.Background(), "stop", "", provider.Usage{})
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestAgentDoneFailsOpenOnTimeout covers ADR-0002's timeout fail-open case
// for the new observer lifecycle point.
func TestAgentDoneFailsOpenOnTimeout(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AgentDone: []config.HookConfig{{Command: "sleep 5; exit 1", TimeoutMs: 50}}},
		Warn:  cw.fn(),
	}
	r.AgentDone(context.Background(), "stop", "", provider.Usage{})
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestAgentDoneFailsOpenOnSignalKill covers ADR-0002's "crashes before
// exiting" fail-open case for the new observer lifecycle point.
func TestAgentDoneFailsOpenOnSignalKill(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AgentDone: []config.HookConfig{{Command: "kill -9 $$"}}},
		Warn:  cw.fn(),
	}
	r.AgentDone(context.Background(), "stop", "", provider.Usage{})
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
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
