package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectStub wires an in-memory client/server pair (the SDK's own testing
// transport — no subprocess, no real MCP server), lets register add tools
// to the server side, and returns the connected client session plus the
// server's own *sdkmcp.Tool definitions as seen from the client (i.e. the
// same shape loadServer/mcpTool see in production).
func connectStub(t *testing.T, register func(*sdkmcp.Server)) (*sdkmcp.ClientSession, []*sdkmcp.Tool) {
	t.Helper()
	ctx := context.Background()

	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "stub", Version: "test"}, nil)
	register(server)
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "liam-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	var defs []*sdkmcp.Tool
	for def, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("session.Tools: %v", err)
		}
		defs = append(defs, def)
	}
	return session, defs
}

// findTool returns the *sdkmcp.Tool named name from defs, failing the test
// if it's missing.
func findTool(t *testing.T, defs []*sdkmcp.Tool, name string) *sdkmcp.Tool {
	t.Helper()
	for _, d := range defs {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("no tool named %q in %+v", name, defs)
	return nil
}
