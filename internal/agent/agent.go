// Package agent implements the agent loop: the core request/tool-call/
// feedback cycle that sends a conversation to a Provider, dispatches any
// requested tool calls to a registered Tool, and threads the results back
// in, repeating until the model stops requesting tools.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
)

// maxStreamAttempts is the total number of times a single turn's
// Provider.Stream call is attempted (the initial attempt plus up to 2
// retries) before a retryable error is surfaced as a final failure.
const maxStreamAttempts = 3

// retryBaseDelay and retryMaxDelay bound the hand-rolled exponential
// backoff between retry attempts: attempt N waits roughly
// min(retryBaseDelay*2^(N-1), retryMaxDelay), jittered. retryMaxDelay is a
// safety cap rather than a delay this reaches in practice: with
// maxStreamAttempts capped at 3, only 2 retry waits ever happen (attempt 1
// then attempt 2), landing in roughly the 0.5s-2s band.
const (
	retryBaseDelay = 1 * time.Second
	retryMaxDelay  = 10 * time.Second
)

// Loop drives one or more Provider turns for a single user message,
// dispatching tool calls against Tools until the model's turn produces no
// further tool calls.
type Loop struct {
	Provider provider.Provider
	Tools    tool.Registry
	// Hooks, when non-nil, gates and observes every tool call via its
	// BeforeTool/AfterTool lifecycle points (see hook.Runner). nil means no
	// hooks are configured.
	Hooks *hook.Runner
	// Backoff computes the delay before the retry attempt that follows a
	// failed attempt numbered attempt (1-indexed). nil uses
	// defaultBackoff, the real jittered exponential backoff; tests override
	// it to avoid real sleeps.
	Backoff func(attempt int) time.Duration
}

// Run sends req, dispatching any ToolCallEvents the Provider yields against
// l.Tools and resending the conversation — including the assistant's
// tool_calls and each tool's Result, threaded in as a matching "tool"-role
// Message — until a turn produces no tool calls. Run derives the advertised
// tool list from l.Tools itself, overriding whatever req.Tools holds.
//
// onEvent, when non-nil, is invoked for every provider.Event yielded across
// every internal turn, letting a caller (TUI, headless) render streamed
// text and tool-call/done bookkeeping without owning the dispatch loop.
//
// Run returns the full message history it built, including req.Messages
// plus every assistant/tool message threaded in along the way.
func (l Loop) Run(ctx context.Context, req provider.Request, onEvent func(provider.Event)) ([]provider.Message, error) {
	messages := append([]provider.Message(nil), req.Messages...)
	tools := toolDefs(l.Tools)

	for {
		turnReq := req
		turnReq.Messages = messages
		turnReq.Tools = tools

		text, calls, err := l.streamTurn(ctx, turnReq, onEvent)
		if err != nil {
			// Preserve whatever text the failing (non-retried) attempt had
			// already streamed — e.g. an Escape-cancelled turn — rather than
			// discarding it; the caller marks the turn
			// "[interrupted]"/"[error: ...]" around whatever partial output
			// survives here.
			if text != "" {
				messages = append(messages, provider.Message{Role: "assistant", Content: text})
			}
			return messages, err
		}

		if len(calls) == 0 {
			if text != "" {
				messages = append(messages, provider.Message{Role: "assistant", Content: text})
			}
			return messages, nil
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   text,
			ToolCalls: calls,
		})
		for _, call := range calls {
			result := l.dispatch(ctx, call)
			if onEvent != nil {
				onEvent(provider.ToolResultEvent{
					ID:       call.ID,
					Name:     call.Name,
					ArgsJSON: call.ArgsJSON,
					Content:  result.Content,
					IsError:  result.IsError,
				})
			}
			messages = append(messages, provider.Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: call.ID,
			})
		}

		// A Tool.Run cancellation (e.g. Escape mid-command) surfaces here as
		// ctx.Err(), not as a Stream error — check it now rather than
		// looping back into a Provider.Stream call already doomed to fail,
		// so cancellation during a tool call is detected at the moment it
		// happens, same as cancellation during Stream itself.
		if err := ctx.Err(); err != nil {
			return messages, err
		}
	}
}

// dispatch runs the Tool named by call against l.Tools, reporting an error
// Result for an unknown tool name or malformed argument JSON rather than
// failing the loop — the model sees the failure and decides how to proceed.
// When l.Hooks is set, a blocking beforeTool hook can deny the call before
// the Tool ever runs (its stderr becomes the error Result the model sees),
// and afterTool hooks observe every call that does run.
func (l Loop) dispatch(ctx context.Context, call provider.ToolCall) tool.Result {
	t, ok := l.Tools[call.Name]
	if !ok {
		return tool.Result{Content: fmt.Sprintf("unknown tool %q", call.Name), IsError: true}
	}

	if l.Hooks != nil {
		if d := l.Hooks.BeforeTool(ctx, call.Name, call.ArgsJSON); d.Blocked {
			return tool.Result{Content: d.Reason, IsError: true}
		}
	}

	var args map[string]any
	if call.ArgsJSON != "" {
		if err := json.Unmarshal([]byte(call.ArgsJSON), &args); err != nil {
			return tool.Result{Content: fmt.Sprintf("invalid arguments for %s: %v", call.Name, err), IsError: true}
		}
	}

	result := t.Run(ctx, args)

	if l.Hooks != nil {
		l.Hooks.AfterTool(ctx, call.Name, call.ArgsJSON, result.Content, result.IsError)
	}

	return result
}

// streamTurn drives one or more Provider.Stream attempts for a single turn,
// applying the ProviderError.Kind-based retry policy: RateLimited and
// Unavailable auto-retry with jittered exponential backoff (up to
// maxStreamAttempts total attempts); every other Kind — including
// ContextTooLong, which ticket 13 (compaction) will change to compact and
// retry once instead — surfaces on the first attempt, like a
// non-ProviderError or an unclassified error would. Retries are invisible
// to the model: a failed attempt's accumulated text/calls are discarded and
// a fresh attempt starts, so only the eventual success or final failure is
// ever threaded into the conversation. onEvent still fires live as each
// attempt streams; a retried attempt is expected to fail before yielding
// any events in practice (a provider-level retryable error surfaces before
// any content), so this doesn't visibly leak a retried attempt's output.
func (l Loop) streamTurn(ctx context.Context, req provider.Request, onEvent func(provider.Event)) (string, []provider.ToolCall, error) {
	for attempt := 1; ; attempt++ {
		var text strings.Builder
		var calls []provider.ToolCall
		var streamErr error

		for ev, err := range l.Provider.Stream(ctx, req) {
			if err != nil {
				streamErr = err
				break
			}
			if onEvent != nil {
				onEvent(ev)
			}
			switch e := ev.(type) {
			case provider.TextDeltaEvent:
				text.WriteString(e.Text)
			case provider.ToolCallEvent:
				calls = append(calls, provider.ToolCall{ID: e.ID, Name: e.Name, ArgsJSON: e.ArgsJSON})
			}
		}

		if streamErr == nil {
			return text.String(), calls, nil
		}
		if attempt >= maxStreamAttempts || !isRetryable(streamErr) {
			return text.String(), calls, streamErr
		}
		if err := waitBackoff(ctx, l.backoffDelay(attempt)); err != nil {
			return text.String(), calls, err
		}
	}
}

// isRetryable reports whether err is a *provider.ProviderError whose Kind
// the retry policy auto-retries.
func isRetryable(err error) bool {
	var perr *provider.ProviderError
	if !errors.As(err, &perr) {
		return false
	}
	switch perr.Kind {
	case provider.ErrorKindRateLimited, provider.ErrorKindUnavailable:
		return true
	case provider.ErrorKindContextTooLong:
		// Ticket 13 (compaction) is this case's extension point: it'll
		// change this to compact the conversation and retry once instead
		// of returning false. For now (ticket 51), treated like Unknown —
		// surface immediately.
		return false
	default:
		return false
	}
}

// backoffDelay returns l.Backoff's result if set, else defaultBackoff's.
func (l Loop) backoffDelay(attempt int) time.Duration {
	if l.Backoff != nil {
		return l.Backoff(attempt)
	}
	return defaultBackoff(attempt)
}

// defaultBackoff computes attempt's hand-rolled exponential backoff delay:
// retryBaseDelay*2^(attempt-1), capped at retryMaxDelay, jittered to a
// random value in [delay/2, delay].
func defaultBackoff(attempt int) time.Duration {
	d := retryBaseDelay * time.Duration(1<<uint(attempt-1))
	if d > retryMaxDelay {
		d = retryMaxDelay
	}
	half := d / 2
	return half + time.Duration(rand.Int63n(int64(d-half+1)))
}

// waitBackoff sleeps for d, returning early with ctx.Err() if ctx is
// canceled first (e.g. Escape mid-backoff).
func waitBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// toolDefs converts a tool.Registry into provider-agnostic ToolDefs, sorted
// by name for deterministic output (map iteration order isn't stable).
func toolDefs(reg tool.Registry) []provider.ToolDef {
	if len(reg) == 0 {
		return nil
	}
	out := make([]provider.ToolDef, 0, len(reg))
	for _, t := range reg {
		out = append(out, provider.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  map[string]any(t.Parameters()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
