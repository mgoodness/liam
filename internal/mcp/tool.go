package mcp

import (
	"context"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/tool"
)

// mcpTool adapts one server-exposed tool into liam's tool.Tool interface,
// dispatching Run through the session it was listed from.
type mcpTool struct {
	session    *sdkmcp.ClientSession
	def        *sdkmcp.Tool
	serverName string
}

func (t *mcpTool) Name() string        { return t.def.Name }
func (t *mcpTool) Description() string { return t.def.Description }

// Parameters returns def.InputSchema as a tool.Schema. From the client
// side, InputSchema already holds the default JSON marshaling of the
// server's schema (a map[string]any, per the SDK's own doc comment); a nil
// schema (a tool with no declared parameters) falls back to an empty
// object schema, matching AddTool's own treatment of an untyped input.
func (t *mcpTool) Parameters() tool.Schema {
	if m, ok := t.def.InputSchema.(map[string]any); ok {
		return tool.Schema(m)
	}
	return tool.Schema{"type": "object"}
}

// Safety reports SideEffectNetwork for every MCP tool — liam has no way to
// know what an out-of-process server's tool actually does, and Network is
// the broadest of the existing SideEffect kinds. This only affects Trace's
// future audit categorization (see ADR-0004), never gating.
func (t *mcpTool) Safety() tool.Safety {
	return tool.Safety{SideEffect: tool.SideEffectNetwork}
}

// Run calls the tool through its originating session, concatenating any
// TextContent blocks in the result via textContent (liam's tool results
// are plain text; non-text content kinds, e.g. images, are silently
// dropped for v1).
func (t *mcpTool) Run(ctx context.Context, args map[string]any) tool.Result {
	res, err := t.session.CallTool(ctx, &sdkmcp.CallToolParams{Name: t.def.Name, Arguments: args})
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("mcp: calling %s (%s): %v", t.def.Name, t.serverName, err), IsError: true}
	}

	return tool.Result{Content: textContent(res.Content), IsError: res.IsError}
}

// textContent concatenates every TextContent block in content —
// mcpTool.Run's plain-text-only convention.
func textContent(content []sdkmcp.Content) string {
	var sb strings.Builder
	for _, c := range content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
