package mcp

import (
	"context"
	"os/exec"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/config"
)

// stubTransport starts an in-memory MCP server (register adds its tools)
// and returns the client-side transport half connected to it. loadServer
// consumes this exactly like a real subprocess transport, minus the
// actual process spawn — the SDK's own testing transport, per issue #48's
// acceptance criteria.
func stubTransport(t *testing.T, register func(*sdkmcp.Server)) sdkmcp.Transport {
	t.Helper()
	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "stub", Version: "test"}, nil)
	register(server)
	if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	return clientTransport
}

func TestLoadServerListsAndWrapsTools(t *testing.T) {
	newTransport := func(config.MCPServerConfig) sdkmcp.Transport {
		return stubTransport(t, func(s *sdkmcp.Server) {
			sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "greet", Description: "say hi"}, greetHandler)
			sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "bye", Description: "say bye"}, greetHandler)
		})
	}

	tools, err := loadServer(context.Background(), "stub", config.MCPServerConfig{}, newTransport)
	if err != nil {
		t.Fatalf("loadServer: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2: %+v", len(tools), tools)
	}
	got := map[string]bool{}
	for _, tl := range tools {
		got[tl.Name()] = true
	}
	if !got["greet"] || !got["bye"] {
		t.Errorf("tool names = %+v, want both greet and bye", got)
	}
}

func TestLoadServerAppliesAllowList(t *testing.T) {
	newTransport := func(config.MCPServerConfig) sdkmcp.Transport {
		return stubTransport(t, func(s *sdkmcp.Server) {
			sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "greet", Description: "say hi"}, greetHandler)
			sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "bye", Description: "say bye"}, greetHandler)
		})
	}

	tools, err := loadServer(context.Background(), "stub", config.MCPServerConfig{Tools: []string{"greet"}}, newTransport)
	if err != nil {
		t.Fatalf("loadServer: %v", err)
	}
	if len(tools) != 1 || tools[0].Name() != "greet" {
		t.Fatalf("tools = %+v, want only [greet]", tools)
	}
}

func TestLoadServerConnectError(t *testing.T) {
	newTransport := func(config.MCPServerConfig) sdkmcp.Transport {
		return &sdkmcp.CommandTransport{Command: exec.Command("/no/such/binary-liam-test")}
	}

	_, err := loadServer(context.Background(), "bad", config.MCPServerConfig{}, newTransport)
	if err == nil {
		t.Fatal("loadServer() error = nil, want a connect error")
	}
}
