// Package trace implements issue #63's Trace: the harness-native, always-on,
// unconfigurable audit log of every tool call's outcome and every hook run.
// It is deliberately not a Hook (ticket "Hooks", #45) — a Hook is
// user-configured and can be left unconfigured entirely, where Trace must
// record every call regardless of what hooks.jsonc says. Trace is instead
// invoked directly by the agent loop's tool-dispatch path (internal/agent's
// Loop.dispatch) and by the hook-execution path (internal/hook's
// Runner.run), and every write is asynchronous and best-effort: a slow disk
// or write error must never block or fail the triggering tool call or hook
// (see Writer).
package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Decision is a tool-call trace line's outcome, per the ticket's spec. There
// is deliberately no "denied_by_permission" value — ADR-0004 removed liam's
// built-in permission system entirely, so a tool call is only ever executed,
// denied by a beforeTool hook, or errored.
type Decision string

const (
	DecisionExecuted     Decision = "executed"
	DecisionDeniedByHook Decision = "denied_by_hook"
	DecisionErrored      Decision = "errored"
)

// stderrCap bounds a HookRunLine's Stderr field, matching the ticket's
// "truncated stderr on failure" acceptance criterion — an unbounded hook
// script's stderr shouldn't be able to bloat the trace file arbitrarily.
const stderrCap = 4096

// ToolCallLine is one JSONL line recording a single tool call's outcome,
// written once that outcome is known (the ticket's first acceptance
// criterion). Every field name is spec'd verbatim by the ticket.
type ToolCallLine struct {
	Timestamp  time.Time `json:"ts"`
	SessionID  string    `json:"session_id"`
	Tool       string    `json:"tool"`
	SideEffect string    `json:"side_effect"`
	Decision   Decision  `json:"decision"`
	// Intent is the model-supplied justification for this call (the
	// ticket's required, schema-injected "intent" property — see
	// internal/agent's withIntent), threaded through unconditionally, not
	// just on denials/errors.
	Intent string `json:"intent,omitempty"`
	// Source names the hook that produced a denied_by_hook Decision (empty
	// otherwise).
	Source string `json:"source,omitempty"`
	// Reason holds the denial or error message, when present.
	Reason string `json:"reason,omitempty"`
	// DurationMs is set only when Decision is DecisionExecuted, per the
	// ticket's "duration_ms (executed only)" criterion — a denied or errored
	// call never actually ran the Tool, so there's no meaningful duration to
	// report.
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// NewToolCallLine builds a ToolCallLine from a dispatch outcome. It's a pure
// function — no I/O — precisely so it can be unit-tested as one (see the
// ticket's "line construction/serialization as pure functions" acceptance
// criterion), independent of Writer's asynchronous file-writing concern.
func NewToolCallLine(now time.Time, sessionID, tool, sideEffect string, decision Decision, intent, source, reason string, duration time.Duration) ToolCallLine {
	l := ToolCallLine{
		Timestamp:  now,
		SessionID:  sessionID,
		Tool:       tool,
		SideEffect: sideEffect,
		Decision:   decision,
		Intent:     intent,
		Source:     source,
		Reason:     reason,
	}
	if decision == DecisionExecuted {
		l.DurationMs = duration.Milliseconds()
	}
	return l
}

// HookRunLine is one JSONL line recording a single hook process's own run —
// its identity, exit code, duration, and truncated stderr on failure —
// separate from any tool-call outcome line it gates (the ticket's second and
// third acceptance criteria).
type HookRunLine struct {
	Timestamp  time.Time `json:"ts"`
	SessionID  string    `json:"session_id"`
	Lifecycle  string    `json:"lifecycle"`
	Command    string    `json:"command"`
	ExitCode   int       `json:"exit_code"`
	DurationMs int64     `json:"duration_ms"`
	Stderr     string    `json:"stderr,omitempty"`
}

// NewHookRunLine builds a HookRunLine from a completed hook run. Like
// NewToolCallLine, it's pure — Stderr truncation is deterministic given its
// inputs, with no I/O involved.
func NewHookRunLine(now time.Time, sessionID, lifecycle, command string, exitCode int, duration time.Duration, stderr string) HookRunLine {
	return HookRunLine{
		Timestamp:  now,
		SessionID:  sessionID,
		Lifecycle:  lifecycle,
		Command:    command,
		ExitCode:   exitCode,
		DurationMs: duration.Milliseconds(),
		Stderr:     truncateStderr(strings.TrimSpace(stderr)),
	}
}

// truncateStderr caps s at stderrCap bytes, backing up to the nearest UTF-8
// rune boundary at or before the cut so a multi-byte rune straddling it is
// dropped whole rather than split into invalid UTF-8 — the same rune-safety
// concern internal/tool's own truncate() handles for tool output.
func truncateStderr(s string) string {
	if len(s) <= stderrCap {
		return s
	}
	cut := stderrCap
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("... [truncated, %d more bytes]", len(s)-cut)
}

// writeQueueSize bounds Writer's internal job queue. Sized generously above
// any realistic tool-call/hook-run burst rate — a full queue only happens if
// the disk itself is pathologically slow, in which case Writer starts
// dropping lines (with a Warn) rather than let the queue grow unbounded or
// block a caller.
const writeQueueSize = 256

// job is one pending write, queued by WriteToolCall/WriteHookRun and
// consumed by Writer's own background goroutine.
type job struct {
	sessionID string
	line      []byte
}

// Writer appends Trace's JSONL lines to
// $XDG_STATE_HOME/liam/traces/<SessionID>.jsonl, switching files whenever
// SessionID changes (e.g. across /clear's fresh session). Every
// WriteToolCall/WriteHookRun call is asynchronous and best-effort: it
// enqueues the line and returns immediately without touching disk itself, so
// a slow disk or write failure can never block or fail the triggering tool
// call or hook. liam ships no config toggle to disable tracing — every call
// site (agent.Loop's dispatch, hook.Runner's run) invokes it
// unconditionally.
type Writer struct {
	// SessionID selects the destination file. It's a plain field, mutated
	// directly by the same call sites that already mutate hook.Runner's own
	// SessionID (cmd/liam/main.go, internal/tui's startSession/`/clear`),
	// matching that field's convention rather than threading a session ID
	// through every WriteToolCall/WriteHookRun call.
	SessionID string
	// Warn, when non-nil, is called with a one-line message whenever a
	// write fails (can't create the traces directory, can't open the file,
	// or the queue is full) — matching hook.Runner's own Warn convention. A
	// write failure never blocks or fails the triggering tool call or hook.
	Warn func(msg string)

	ch   chan job
	done chan struct{}

	// file/openFor are owned exclusively by the background goroutine
	// started in New (loop) — never touched from any other goroutine, so
	// they need no lock.
	file    *os.File
	openFor string
}

// New starts Writer's background goroutine and returns a Writer ready to
// receive writes. The traces directory need not exist yet — it's created on
// the first write.
func New() *Writer {
	w := &Writer{
		ch:   make(chan job, writeQueueSize),
		done: make(chan struct{}),
	}
	go w.loop()
	return w
}

// WriteToolCall enqueues a ToolCallLine (see NewToolCallLine) built from its
// arguments, tagged with w.SessionID at the moment of the call.
func (w *Writer) WriteToolCall(tool, sideEffect string, decision Decision, intent, source, reason string, duration time.Duration) {
	sessionID := w.SessionID
	w.emit(sessionID, NewToolCallLine(time.Now().UTC(), sessionID, tool, sideEffect, decision, intent, source, reason, duration))
}

// WriteHookRun enqueues a HookRunLine (see NewHookRunLine) built from its
// arguments, tagged with w.SessionID at the moment of the call.
func (w *Writer) WriteHookRun(lifecycle, command string, exitCode int, duration time.Duration, stderr string) {
	sessionID := w.SessionID
	w.emit(sessionID, NewHookRunLine(time.Now().UTC(), sessionID, lifecycle, command, exitCode, duration, stderr))
}

// Close stops Writer's background goroutine and closes whatever trace file
// is currently open, waiting for every already-enqueued write to finish
// first. Close must only be called once every WriteToolCall/WriteHookRun
// call has already returned — true for every call site in this codebase,
// since dispatch/hook.run invoke Writer synchronously (enqueuing is
// synchronous; only the disk write itself is asynchronous) before their own
// caller can reach program shutdown and call Close.
func (w *Writer) Close() {
	close(w.ch)
	<-w.done
}

// emit marshals v and enqueues it, tagged with sessionID. A marshaling
// failure (can't happen for either Line type as built by this package, but
// could in principle for a future field) only reaches Warn.
func (w *Writer) emit(sessionID string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		w.warn(fmt.Sprintf("trace: encoding line: %v", err))
		return
	}
	w.enqueue(sessionID, append(data, '\n'))
}

// enqueue sends line to w.ch without blocking: a full queue drops the line
// (with a Warn) rather than block the caller, satisfying Writer's
// asynchronous/best-effort contract even under pathological backpressure.
func (w *Writer) enqueue(sessionID string, line []byte) {
	select {
	case w.ch <- job{sessionID: sessionID, line: line}:
	default:
		w.warn("trace: dropped a line, write queue full")
	}
}

// loop is Writer's background goroutine: the sole owner of w.file/w.openFor,
// so switching files on a session change needs no synchronization with
// WriteToolCall/WriteHookRun, which only ever touch the channel.
func (w *Writer) loop() {
	defer close(w.done)
	for j := range w.ch {
		if err := w.writeLine(j.sessionID, j.line); err != nil {
			w.warn(fmt.Sprintf("trace: %v", err))
		}
	}
	if w.file != nil {
		_ = w.file.Close()
	}
}

// writeLine appends line to sessionID's trace file, opening (and, on a
// session change, first closing the previous) file as needed.
func (w *Writer) writeLine(sessionID string, line []byte) error {
	if w.file == nil || w.openFor != sessionID {
		if w.file != nil {
			_ = w.file.Close()
		}
		dir, err := tracesDir()
		if err != nil {
			return fmt.Errorf("locating state directory: %w", err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating traces directory: %w", err)
		}
		f, err := os.OpenFile(filepath.Join(dir, sessionID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("opening trace file: %w", err)
		}
		w.file = f
		w.openFor = sessionID
	}
	_, err := w.file.Write(line)
	return err
}

func (w *Writer) warn(msg string) {
	if w.Warn != nil {
		w.Warn(msg)
	}
}
