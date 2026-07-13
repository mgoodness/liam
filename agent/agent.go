// Package agent implements the agent loop: send the running message
// history to a Provider, execute any tool calls in its response via the
// tool dispatch table, feed the results back, and repeat until a response
// contains no tool calls.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// Callbacks lets a caller observe agent.Run's progress synchronously, at
// the point each event happens, without affecting control flow: no field
// gates or delays a tool call, so YOLO mode is unaffected (see ADR 0006).
// Any nil field is simply not invoked.
type Callbacks struct {
	// OnText fires whenever a response carries text, whether or not it
	// also carries tool calls — including the final, tool-call-free
	// response.
	OnText func(text string)

	// OnToolCall fires immediately before a tool call executes.
	OnToolCall func(name string, args json.RawMessage)

	// OnToolResult fires immediately after a tool call executes, with the
	// same (already-truncated) result string that gets fed back into
	// history, so what's reported here matches what the model itself
	// sees. err is the tool-level error, if any; result already
	// incorporates it (formatted as "error: ..."). Does not fire if the
	// call was abandoned mid-flight by a cancelled context — OnToolCall
	// may fire for a call whose OnToolResult never comes.
	OnToolResult func(name, result string, err error)
}

// Run drives the agent loop against p, starting from history and executing
// tool calls via tool.Call against tools — the same set (typically built
// by tool.New) whose Definitions were offered to p, so every name p can
// request resolves here. It returns the final assistant text once a
// response contains no tool calls, along with the full updated history
// (including every appended assistant and tool-result message) so the
// caller can continue the conversation. cb's fields, if set, are invoked
// synchronously as each event happens.
func Run(ctx context.Context, p provider.Provider, tools map[string]tool.Tool, history []provider.Message, cb Callbacks) (string, []provider.Message, error) {
	// Own copy: append below must never write into spare capacity of the
	// caller's backing array, which would mutate their slice out from
	// under them.
	history = append([]provider.Message(nil), history...)

	for {
		if err := ctx.Err(); err != nil {
			return "", history, err
		}

		resp, err := p.Complete(ctx, history)
		if err != nil {
			return "", history, err
		}
		history = append(history, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
		})

		if resp.Text != "" && cb.OnText != nil {
			cb.OnText(resp.Text)
		}

		if len(resp.ToolCalls) == 0 {
			return resp.Text, history, nil
		}

		for _, call := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				return "", history, err
			}

			if cb.OnToolCall != nil {
				cb.OnToolCall(call.Name, call.Arguments)
			}

			result, err := tool.Call(ctx, tools, call.Name, call.Arguments)
			if errors.Is(err, tool.ErrUnknownTool) {
				return "", history, err
			}
			// A tool call abandoned mid-flight by cancellation gets no
			// synthetic result appended — it's not a tool-level failure to
			// relay to the model, and the model never sees this turn.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", history, ctxErr
			}
			if err != nil {
				result = fmt.Sprintf("error: %s", err)
			}

			if cb.OnToolResult != nil {
				cb.OnToolResult(call.Name, result, err)
			}

			history = append(history, provider.Message{
				Role:       provider.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}
}
