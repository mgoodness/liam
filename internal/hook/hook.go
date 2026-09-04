// Package hook dispatches liam's 4 hook lifecycle points — sessionStart,
// sessionEnd, beforeTool, afterTool — against a config.HooksConfig, running
// each matching hook's command as a child process. A blocking (non-async)
// beforeTool hook can gate the tool call it wraps: a non-zero exit denies
// the call, with the hook's stderr surfaced to the model as the reason
// (see ADR-0002 for the fail-open rules governing everything else).
package hook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/shellrun"
	"github.com/mgoodness/liam/internal/trace"
)

// Lifecycle identifies one of the 4 hook lifecycle points.
type Lifecycle string

const (
	SessionStart Lifecycle = "sessionStart"
	SessionEnd   Lifecycle = "sessionEnd"
	BeforeTool   Lifecycle = "beforeTool"
	AfterTool    Lifecycle = "afterTool"
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
	// records for the call a beforeTool hook gates. nil disables tracing
	// (e.g. in tests that don't construct one), never the hooks themselves.
	Trace *trace.Writer
}

// Decision is BeforeTool's verdict: whether a blocking hook denied the call,
// and why.
type Decision struct {
	Blocked bool
	// Reason is the denying hook's stderr (or a fallback describing the
	// exit code, if stderr was empty), surfaced to the model as the tool
	// result.
	Reason string
	// Source is the denying hook's Command, threaded through to Trace's
	// per-tool-call "source" field by the caller. Empty when Blocked is
	// false.
	Source string
}

// SessionStart runs every configured sessionStart hook.
func (r *Runner) SessionStart(ctx context.Context) {
	r.dispatch(ctx, SessionStart, r.Hooks.SessionStart, nil, nil)
}

// SessionEnd runs every configured sessionEnd hook.
func (r *Runner) SessionEnd(ctx context.Context) {
	r.dispatch(ctx, SessionEnd, r.Hooks.SessionEnd, nil, nil)
}

// BeforeTool runs every configured beforeTool hook matching name, in
// declaration order, stopping at the first blocking (non-async) hook that
// exits non-zero after actually running. An async hook is fired and never
// gates the call, matching AfterTool's fire-and-forget precedent.
func (r *Runner) BeforeTool(ctx context.Context, name, argsJSON string) Decision {
	ti := &toolInfo{Name: name, Args: rawArgs(argsJSON)}

	for _, hc := range r.Hooks.BeforeTool {
		if !matches(hc.Match, name) {
			continue
		}
		if hc.Async {
			go r.runAndWarn(context.Background(), hc, BeforeTool, ti, nil)
			continue
		}

		oc := r.run(ctx, hc, BeforeTool, ti, nil)
		if oc.err != nil {
			r.warnFailOpen(hc, BeforeTool, oc.err)
			continue
		}
		if oc.exitCode != 0 {
			reason := strings.TrimSpace(oc.stderr)
			if reason == "" {
				reason = fmt.Sprintf("blocked by hook %q (exit %d)", hc.Command, oc.exitCode)
			}
			return Decision{Blocked: true, Reason: reason, Source: hc.Command}
		}
	}
	return Decision{}
}

// AfterTool runs every configured afterTool hook matching name. It never
// gates anything — a non-zero exit is only logged via Warn.
func (r *Runner) AfterTool(ctx context.Context, name, argsJSON, content string, isError bool) {
	ti := &toolInfo{Name: name, Args: rawArgs(argsJSON)}
	ri := &resultInfo{Content: content, IsError: isError}
	r.dispatch(ctx, AfterTool, r.Hooks.AfterTool, ti, ri)
}

// dispatch runs every hook in hooks matching ti (ti == nil, as with
// sessionStart/sessionEnd, always matches), respecting each hook's Async
// flag. None of these lifecycle points can gate anything — failures only
// reach Warn.
func (r *Runner) dispatch(ctx context.Context, lc Lifecycle, hooks []config.HookConfig, ti *toolInfo, ri *resultInfo) {
	for _, hc := range hooks {
		if ti != nil && !matches(hc.Match, ti.Name) {
			continue
		}
		if hc.Async {
			go r.runAndWarn(context.Background(), hc, lc, ti, ri)
			continue
		}
		r.runAndWarn(ctx, hc, lc, ti, ri)
	}
}

// runAndWarn runs hc and reports any failure (fail-open condition or a
// non-zero exit) via Warn.
func (r *Runner) runAndWarn(ctx context.Context, hc config.HookConfig, lc Lifecycle, ti *toolInfo, ri *resultInfo) {
	oc := r.run(ctx, hc, lc, ti, ri)
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
// never for a plain non-zero exit, which is a normal, gate-eligible return
// captured in exitCode/stderr instead.
type outcome struct {
	exitCode int
	stdout   string
	stderr   string
	err      error
}

// run executes hc's command as a child process, feeding it lc's stdin JSON
// payload and parallel LIAM_* environment variables. Every call — success,
// denial, or fail-open condition alike — records issue #63's HookRunLine via
// r.Trace, via the deferred closure below: `return outcome{...}` still
// assigns the named oc result before the deferred func runs, so every one of
// run's early-return branches is covered without repeating the trace call at
// each one.
func (r *Runner) run(ctx context.Context, hc config.HookConfig, lc Lifecycle, ti *toolInfo, ri *resultInfo) (oc outcome) {
	start := time.Now()
	defer func() {
		if r.Trace != nil {
			r.Trace.WriteHookRun(string(lc), hc.Command, oc.exitCode, time.Since(start), oc.stderr)
		}
	}()

	payload := struct {
		Lifecycle string      `json:"lifecycle"`
		SessionID string      `json:"sessionId"`
		Cwd       string      `json:"cwd"`
		Tool      *toolInfo   `json:"tool,omitempty"`
		Result    *resultInfo `json:"result,omitempty"`
	}{
		Lifecycle: string(lc),
		SessionID: r.SessionID,
		Cwd:       r.Cwd,
		Tool:      ti,
		Result:    ri,
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

	res := shellrun.Run(runCtx, hc.Command, stdin, r.Cwd, envFor(lc, r.SessionID, r.Cwd, ti))

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
