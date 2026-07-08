// Package agent implements the agent loop: send the running message
// history to a Provider, execute any tool calls in its response via the
// tool dispatch table, feed the results back, and repeat until a response
// contains no tool calls.
package agent

import (
	"context"
	"fmt"

	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// Run drives the agent loop against p, starting from history, executing
// tool calls via tools. It returns the final assistant text once a
// response contains no tool calls, along with the full updated history
// (including every appended assistant and tool-result message) so the
// caller can continue the conversation.
func Run(ctx context.Context, p provider.Provider, tools map[string]tool.Tool, history []provider.Message) (string, []provider.Message, error) {
	// Own copy: append below must never write into spare capacity of the
	// caller's backing array, which would mutate their slice out from
	// under them.
	history = append([]provider.Message(nil), history...)

	for {
		resp, err := p.Complete(ctx, history)
		if err != nil {
			return "", history, err
		}
		history = append(history, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
		})

		if len(resp.ToolCalls) == 0 {
			return resp.Text, history, nil
		}

		for _, call := range resp.ToolCalls {
			result, err := callTool(ctx, tools, call)
			if err != nil {
				return "", history, err
			}
			history = append(history, provider.Message{
				Role:       provider.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}
}

// callTool invokes call via tools and returns the text to feed back to the
// model as the tool's result: the tool's own output on success, or a
// description of the failure on error — a failing tool call is reported
// to the model, not the caller, so the loop keeps running. An unknown
// tool name is instead returned as a Go error, mirroring tool.Call's
// contract that this signals the Provider requesting a tool outside the
// definitions it was given, not a tool-execution failure to relay back.
func callTool(ctx context.Context, tools map[string]tool.Tool, call provider.ToolCall) (string, error) {
	t, ok := tools[call.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
	result, err := t.Handler(ctx, call.Arguments)
	if err != nil {
		return fmt.Sprintf("error: %s", err), nil
	}
	return tool.Truncate(result), nil
}
