// Package hook dispatches liam's 4 hook lifecycle points — sessionStart,
// sessionEnd, afterTool, agentDone — against a config.HooksConfig, running
// each matching hook's command as a child process. Every lifecycle point is
// a pure observer: none can gate or deny anything, only log a failure via
// Warn (see ADR-0002 for the fail-open rules governing hook-process
// failures specifically). liam previously shipped a blocking beforeTool/
// userPromptSubmit contract (issues #84, #102); both were removed after
// shipping with zero real configured usage — see ADR-0004's note on tool-
// call gating.
package hook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/shellrun"
	"github.com/mgoodness/liam/internal/trace"
)

// Lifecycle identifies one of the 4 hook lifecycle points.
type Lifecycle string

const (
	SessionStart Lifecycle = "sessionStart"
	SessionEnd   Lifecycle = "sessionEnd"
	AfterTool    Lifecycle = "afterTool"
	AgentDone    Lifecycle = "agentDone"
)

// Runner dispatches hooks configured under one HooksConfig for a single
// session.
type Runner struct {
	Hooks     config.HooksConfig
	SessionID string
	Cwd       string
	// Warn, when non-nil, is called with a one-line message for every
	// fail-open condition (ADR-0002: timeout, crash, command not found) and
	// for a non-blocking hook's non-zero exit — so a caller can surface it
	// (e.g. to stderr) without the Runner owning any particular logging
	// destination.
	Warn func(msg string)
	// Trace, when non-nil, records issue #63's per-run audit line for every
	// hook this Runner runs (see run) — separate from, and regardless of,
	// any tool-call outcome line the caller (internal/agent's Loop.dispatch)
	// records for the same call. nil disables tracing (e.g. in tests that
	// don't construct one), never the hooks themselves.
	Trace *trace.Writer
}

// SessionStart runs every configured sessionStart hook.
func (r *Runner) SessionStart(ctx context.Context) {
	r.dispatch(ctx, SessionStart, r.Hooks.SessionStart, extras{})
}

// SessionEnd runs every configured sessionEnd hook.
func (r *Runner) SessionEnd(ctx context.Context) {
	r.dispatch(ctx, SessionEnd, r.Hooks.SessionEnd, extras{})
}

// AfterTool runs every configured afterTool hook matching name. It never
// gates anything — a non-zero exit is only logged via Warn.
func (r *Runner) AfterTool(ctx context.Context, name, argsJSON, content string, isError bool) {
	ex := extras{
		Tool:   &toolInfo{Name: name, Args: rawArgs(argsJSON)},
		Result: &resultInfo{Content: content, IsError: isError},
	}
	r.dispatch(ctx, AfterTool, r.Hooks.AfterTool, ex)
}

// AgentDone runs every configured agentDone hook once per Agent loop
// invocation concluding (the caller — internal/agent's Loop.Run — is
// responsible for firing this exactly once per Run call, not per individual
// provider turn within a multi-tool-call loop). Pure observer, matching
// AfterTool's fire-and-forget contract: it never gates anything, and a
// non-zero exit is only logged via Warn.
func (r *Runner) AgentDone(ctx context.Context, finishReason, modelUsed string, usage provider.Usage) {
	ex := extras{Done: &doneInfo{FinishReason: finishReason, ModelUsed: modelUsed, Usage: usage}}
	r.dispatch(ctx, AgentDone, r.Hooks.AgentDone, ex)
}

// extras bundles the lifecycle-point-specific fields of a hook's stdin JSON
// payload, so dispatch/runAndWarn/run take a single value instead of a
// three-pointer parameter list that would otherwise be nil at nearly every
// call site: every lifecycle point but AfterTool (which sets both Tool and
// Result) sets at most one field, the rest left nil. json.Marshal flattens
// its fields inline into run's payload struct via Go's anonymous-field
// embedding.
type extras struct {
	Tool   *toolInfo   `json:"tool,omitempty"`
	Result *resultInfo `json:"result,omitempty"`
	Done   *doneInfo   `json:"done,omitempty"`
}

// dispatch runs every hook in hooks matching ex.Tool (ex.Tool == nil, as
// with sessionStart/sessionEnd/agentDone, always matches), respecting each
// hook's Async flag. No lifecycle point dispatched this way can gate
// anything — failures only reach Warn.
func (r *Runner) dispatch(ctx context.Context, lc Lifecycle, hooks []config.HookConfig, ex extras) {
	for _, hc := range hooks {
		if ex.Tool != nil && !matches(hc.Match, ex.Tool.Name) {
			continue
		}
		if hc.Async {
			go r.runAndWarn(context.Background(), hc, lc, ex)
			continue
		}
		r.runAndWarn(ctx, hc, lc, ex)
	}
}

// runAndWarn runs hc and reports any failure (fail-open condition or a
// non-zero exit) via Warn.
func (r *Runner) runAndWarn(ctx context.Context, hc config.HookConfig, lc Lifecycle, ex extras) {
	oc := r.run(ctx, hc, lc, ex)
	if oc.err != nil {
		r.warnFailOpen(hc, lc, oc.err)
		return
	}
	if oc.exitCode != 0 {
		r.warn(fmt.Sprintf("hook %q (%s) exited %d: %s", hc.Command, lc, oc.exitCode, strings.TrimSpace(oc.stderr)))
	}
}

func (r *Runner) warnFailOpen(hc config.HookConfig, lc Lifecycle, err error) {
	r.warn(fmt.Sprintf("hook %q (%s): %v — failing open", hc.Command, lc, err))
}

func (r *Runner) warn(msg string) {
	if r.Warn != nil {
		r.Warn(msg)
	}
}

// outcome is one hook process's result. err is set only for a fail-open
// condition (ADR-0002: the command couldn't be started, or it timed out) —
// never for a plain non-zero exit, which is a normal (if failing) return
// captured in exitCode/stderr instead.
type outcome struct {
	exitCode int
	stdout   string
	stderr   string
	err      error
}

// run executes hc's command as a child process, feeding it lc's stdin JSON
// payload and parallel LIAM_* environment variables. Every call — success,
// failure, or fail-open condition alike — records issue #63's HookRunLine
// via r.Trace, via the deferred closure below: `return outcome{...}` still
// assigns the named oc result before the deferred func runs, so every one of
// run's early-return branches is covered without repeating the trace call at
// each one.
func (r *Runner) run(ctx context.Context, hc config.HookConfig, lc Lifecycle, ex extras) (oc outcome) {
	start := time.Now()
	defer func() {
		if r.Trace != nil {
			r.Trace.WriteHookRun(string(lc), hc.Command, oc.exitCode, time.Since(start), oc.stderr)
		}
	}()

	payload := struct {
		Lifecycle string `json:"lifecycle"`
		SessionID string `json:"sessionId"`
		Cwd       string `json:"cwd"`
		extras
	}{
		Lifecycle: string(lc),
		SessionID: r.SessionID,
		Cwd:       r.Cwd,
		extras:    ex,
	}
	stdin, err := json.Marshal(payload)
	if err != nil {
		return outcome{err: fmt.Errorf("encoding hook payload: %w", err)}
	}

	runCtx := ctx
	if hc.TimeoutMs > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(hc.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	res := shellrun.Run(runCtx, hc.Command, stdin, r.Cwd, envFor(lc, r.SessionID, r.Cwd, ex.Tool))

	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return outcome{exitCode: res.ExitCode, stderr: res.Stderr, err: fmt.Errorf("timed out after %dms", hc.TimeoutMs)}
	}
	if res.Err != nil {
		// Couldn't even start sh itself — no real exit code to report.
		return outcome{exitCode: -1, err: res.Err}
	}
	// sh's own 127/126 convention for "command not found"/"found but not
	// executable" is itself a fail-open condition (ADR-0002's "whose
	// command can't be found"), not a policy verdict — sh never got to
	// run the configured command at all.
	if code := res.ExitCode; code == 127 || code == 126 {
		return outcome{exitCode: res.ExitCode, stderr: res.Stderr, err: fmt.Errorf("command not found or not executable (exit %d): %s", code, strings.TrimSpace(res.Stderr))}
	}
	// ExitCode() reports -1 when the process was terminated by a signal
	// rather than returning normally (os/exec's documented behavior) —
	// ADR-0002's "crashes before exiting" fail-open case, not a policy
	// verdict.
	if res.ExitCode == -1 {
		return outcome{exitCode: -1, stderr: res.Stderr, err: fmt.Errorf("hook process terminated abnormally: %s", strings.TrimSpace(res.Stderr))}
	}
	// Otherwise the process ran and returned — a non-zero exit is a real
	// verdict, not a fail-open condition.
	return outcome{exitCode: res.ExitCode, stdout: res.Stdout, stderr: res.Stderr}
}

// toolInfo is the "tool" field of a hook's stdin JSON payload.
type toolInfo struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// resultInfo is the "result" field of an afterTool hook's stdin JSON
// payload.
type resultInfo struct {
	Content string `json:"content"`
	IsError bool   `json:"isError"`
}

// doneInfo is the "done" field of an agentDone hook's stdin JSON payload,
// mirroring provider.DoneEvent's fields for the Agent loop invocation that
// just concluded.
type doneInfo struct {
	FinishReason string         `json:"finishReason"`
	ModelUsed    string         `json:"modelUsed"`
	Usage        provider.Usage `json:"usage"`
}

func rawArgs(argsJSON string) json.RawMessage {
	if argsJSON == "" {
		return nil
	}
	return json.RawMessage(argsJSON)
}

// matches reports whether name is covered by patterns: an empty patterns
// list, or a literal "*" entry, matches everything.
func matches(patterns []string, name string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if p == "*" || p == name {
			return true
		}
	}
	return false
}

// envFor builds the LIAM_* environment variables run parallel to the stdin
// JSON payload, plus LIAM_HOOKS_DISABLED=1 (ADR-0002) so a hook that itself
// invokes liam headlessly doesn't recursively re-trigger hooks.
func envFor(lc Lifecycle, sessionID, cwd string, ti *toolInfo) []string {
	env := []string{
		"LIAM_LIFECYCLE=" + string(lc),
		"LIAM_SESSION_ID=" + sessionID,
		"LIAM_CWD=" + cwd,
		"LIAM_HOOKS_DISABLED=1",
	}
	if ti != nil {
		env = append(env, "LIAM_TOOL_NAME="+ti.Name)
	}
	return env
}
