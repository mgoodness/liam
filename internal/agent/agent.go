// Package agent implements the agent loop: the core request/tool-call/
// feedback cycle that sends a conversation to a Provider, dispatches any
// requested tool calls to a registered Tool, and threads the results back
// in, repeating until the model stops requesting tools.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
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

		var text strings.Builder
		var calls []provider.ToolCall

		for ev, err := range l.Provider.Stream(ctx, turnReq) {
			if err != nil {
				// Preserve whatever text this turn had already streamed
				// (e.g. an Escape-cancelled turn) rather than discarding
				// it — the caller marks the turn "[interrupted]"/"[error:
				// ...]" around whatever partial output survives here.
				if text.Len() > 0 {
					messages = append(messages, provider.Message{Role: "assistant", Content: text.String()})
				}
				return messages, err
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

		if len(calls) == 0 {
			if text.Len() > 0 {
				messages = append(messages, provider.Message{Role: "assistant", Content: text.String()})
			}
			return messages, nil
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   text.String(),
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
