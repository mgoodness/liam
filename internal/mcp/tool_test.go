package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/tool"
)

func greetHandler(_ context.Context, _ *sdkmcp.CallToolRequest, in map[string]any) (*sdkmcp.CallToolResult, any, error) {
	name, _ := in["name"].(string)
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "hello, " + name}},
	}, nil, nil
}

func failHandler(_ context.Context, _ *sdkmcp.CallToolRequest, _ map[string]any) (*sdkmcp.CallToolResult, any, error) {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "something went wrong"}},
		IsError: true,
	}, nil, nil
}

func TestMCPToolNameDescriptionFromDef(t *testing.T) {
	session, defs := connectStub(t, func(s *sdkmcp.Server) {
		sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "greet", Description: "say hi"}, greetHandler)
	})
	def := findTool(t, defs, "greet")

	mt := &mcpTool{session: session, def: def, serverName: "stub"}

	if mt.Name() != "greet" {
		t.Errorf("Name() = %q, want greet", mt.Name())
	}
	if mt.Description() != "say hi" {
		t.Errorf("Description() = %q, want %q", mt.Description(), "say hi")
	}
}

func TestMCPToolParametersFromInputSchema(t *testing.T) {
	session, defs := connectStub(t, func(s *sdkmcp.Server) {
		sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "greet", Description: "say hi"}, greetHandler)
	})
	def := findTool(t, defs, "greet")

	mt := &mcpTool{session: session, def: def, serverName: "stub"}

	params := mt.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters()[\"type\"] = %v, want %q (inferred from the map[string]any handler input)", params["type"], "object")
	}
}

func TestMCPToolSafetyIsNetwork(t *testing.T) {
	mt := &mcpTool{serverName: "stub"}
	want := tool.Safety{SideEffect: tool.SideEffectNetwork}
	if got := mt.Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestMCPToolRunReturnsTextContent(t *testing.T) {
	session, defs := connectStub(t, func(s *sdkmcp.Server) {
		sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "greet", Description: "say hi"}, greetHandler)
	})
	def := findTool(t, defs, "greet")
	mt := &mcpTool{session: session, def: def, serverName: "stub"}

	got := mt.Run(context.Background(), map[string]any{"name": "world"})

	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if got.Content != "hello, world" {
		t.Errorf("Content = %q, want %q", got.Content, "hello, world")
	}
}

func TestMCPToolRunSurfacesToolError(t *testing.T) {
	session, defs := connectStub(t, func(s *sdkmcp.Server) {
		sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "fail", Description: "always fails"}, failHandler)
	})
	def := findTool(t, defs, "fail")
	mt := &mcpTool{session: session, def: def, serverName: "stub"}

	got := mt.Run(context.Background(), map[string]any{})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
	if got.Content != "something went wrong" {
		t.Errorf("Content = %q, want %q", got.Content, "something went wrong")
	}
}

func TestMCPToolRunSurfacesCallError(t *testing.T) {
	session, _ := connectStub(t, func(*sdkmcp.Server) {})
	// def names a tool the server never registered, so CallTool itself
	// fails at the protocol level (not a tool-reported error).
	def := &sdkmcp.Tool{Name: "nonexistent"}
	mt := &mcpTool{session: session, def: def, serverName: "stub"}

	got := mt.Run(context.Background(), map[string]any{})

	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}
