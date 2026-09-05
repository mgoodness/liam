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

// TestRunReceivesStdinJSONAndLIAMEnvVars drives a real "sh -c" hook (via
// AfterTool) that writes its stdin JSON (line 1) and the LIAM_* env vars it
// saw (remaining lines) to a temp file, then reads the file back to assert
// on both.
func TestRunReceivesStdinJSONAndLIAMEnvVars(t *testing.T) {
	dir := t.TempDir()
	outPath := dir + "/hook-saw.json"
	cmd := `{ cat; echo; } > ` + outPath + `; { echo "LIFECYCLE=$LIAM_LIFECYCLE"; echo "SESSION=$LIAM_SESSION_ID"; echo "CWD=$LIAM_CWD"; echo "TOOL=$LIAM_TOOL_NAME"; echo "DISABLED=$LIAM_HOOKS_DISABLED"; } >> ` + outPath

	r := &Runner{
		Hooks:     config.HooksConfig{AfterTool: []config.HookConfig{{Command: cmd}}},
		SessionID: "sess-123",
		Cwd:       dir,
	}
	r.AfterTool(context.Background(), "bash", `{"command":"ls"}`, "output", false)

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
	if stdin.Lifecycle != "afterTool" {
		t.Errorf("stdin lifecycle = %q, want afterTool", stdin.Lifecycle)
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

	if !strings.Contains(got, "LIFECYCLE=afterTool") {
		t.Errorf("captured env = %q, want LIAM_LIFECYCLE=afterTool", got)
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

// TestAfterToolMatchRestrictsToNamedTools covers AfterTool's Match filter —
// the one remaining lifecycle point where Match still means something now
// that beforeTool is gone.
func TestAfterToolMatchRestrictsToNamedTools(t *testing.T) {
	dir := t.TempDir()
	ranPath := dir + "/ran"
	r := &Runner{Hooks: config.HooksConfig{
		AfterTool: []config.HookConfig{{Command: "touch " + ranPath, Match: []string{"edit"}}},
	}}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	if fileExists(ranPath) {
		t.Error("afterTool hook ran for \"bash\", want it restricted to [edit]")
	}
}

// TestAfterToolFailsOpenOnTimeout covers ADR-0002's fail-open contract for
// the one remaining lifecycle point with a timeout-prone real use case
// (afterTool is the most likely to run a slow external command).
func TestAfterToolFailsOpenOnTimeout(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: "sleep 5; exit 1", TimeoutMs: 50}}},
		Warn:  cw.fn(),
	}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestAfterToolFailsOpenOnCommandNotFound covers ADR-0002's other fail-open
// case for afterTool, matching AgentDone's equivalent coverage.
func TestAfterToolFailsOpenOnCommandNotFound(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: "/no/such/binary-liam-test"}}},
		Warn:  cw.fn(),
	}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
	}
}

// TestAfterToolFailsOpenOnSignalKill covers ADR-0002's "crashes before
// exiting" fail-open case for afterTool, matching AgentDone's equivalent
// coverage.
func TestAfterToolFailsOpenOnSignalKill(t *testing.T) {
	cw := &captureWarnings{}
	r := &Runner{
		Hooks: config.HooksConfig{AfterTool: []config.HookConfig{{Command: "kill -9 $$"}}},
		Warn:  cw.fn(),
	}
	r.AfterTool(context.Background(), "bash", `{}`, "output", false)
	if len(cw.all()) == 0 {
		t.Error("Warn was never called, want a fail-open warning")
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

// TestAgentDoneFailsOpenOnCommandNotFound covers ADR-0002 for an observer
// lifecycle point.
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
// for an observer lifecycle point.
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
// exiting" fail-open case for an observer lifecycle point.
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

// TestAsyncHookNeverBlocksCaller covers async: true's fire-and-forget
// contract in general: an async hook runs in the background rather than
// blocking its dispatching call, regardless of lifecycle point.
func TestAsyncHookNeverBlocksCaller(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	r := &Runner{Hooks: config.HooksConfig{
		AfterTool: []config.HookConfig{{Command: "exit 1", Async: true}},
	}}
	r.Warn = func(string) { wg.Done() }

	r.AfterTool(context.Background(), "bash", `{}`, "output", false)

	waitOrTimeout(t, &wg)
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
