// Package agent implements the agent loop: send the running message
// history to a Provider, execute any tool calls in its response via the
// tool dispatch table, feed the results back, and repeat until a response
// contains no tool calls.
package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// Run drives the agent loop against p, starting from history and executing
// tool calls via the tool package's dispatch table (tool.Call). It returns
// the final assistant text once a response contains no tool calls, along
// with the full updated history (including every appended assistant and
// tool-result message) so the caller can continue the conversation.
func Run(ctx context.Context, p provider.Provider, history []provider.Message) (string, []provider.Message, error) {
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
			result, err := tool.Call(ctx, call.Name, call.Arguments)
			if errors.Is(err, tool.ErrUnknownTool) {
				return "", history, err
			}
			if err != nil {
				result = fmt.Sprintf("error: %s", err)
			}
			history = append(history, provider.Message{
				Role:       provider.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}
}
