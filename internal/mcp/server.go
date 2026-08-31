package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/tool"
)

// newCommandTransport builds the real, production stdio-subprocess
// Transport for a configured server. Tests inject a different transport
// factory (an in-memory pair) directly into loadServer, so this is the
// only code path that ever spawns a real process.
func newCommandTransport(sc config.MCPServerConfig) sdkmcp.Transport {
	cmd := exec.Command(sc.Command, sc.Args...)
	cmd.Env = append(os.Environ(), expandEnv(sc.Env)...)
	return &sdkmcp.CommandTransport{Command: cmd}
}

// expandEnv turns a config.MCPServerConfig's Env map into "KEY=value"
// pairs, $VAR-expanding each value against the harness's own environment.
func expandEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+os.ExpandEnv(v))
	}
	return out
}

// clientCapabilities declares liam's own MCP client capabilities
// (issue #48's "declaring the client's own roots capability"): roots,
// deliberately, without any actual root entries (Client.AddRoots is never
// called — liam has no specific directory it wants to expose this way yet,
// and the roots feature itself is deprecated per SEP-2577). Declared
// explicitly rather than relying on NewClient's own nil-Capabilities
// default (which happens to advertise roots too, "for historical
// reasons") so this is a deliberate choice, not an accidental byproduct.
var clientCapabilities = &sdkmcp.ClientCapabilities{RootsV2: &sdkmcp.RootCapabilities{}}

// loadServer connects to one server via newTransport, lists its tools
// (applying sc.Tools as an allow-list when non-empty), and returns them
// wrapped as tool.Tools sharing the one session — the session stays open
// for the tools' lifetime, closed only if listing itself fails.
func loadServer(ctx context.Context, name string, sc config.MCPServerConfig, newTransport func(config.MCPServerConfig) sdkmcp.Transport) ([]tool.Tool, error) {
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "liam", Version: "dev"}, &sdkmcp.ClientOptions{Capabilities: clientCapabilities})
	session, err := client.Connect(ctx, newTransport(sc), nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connecting to %q: %w", name, err)
	}

	var tools []tool.Tool
	for def, err := range session.Tools(ctx, nil) {
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("mcp: listing tools from %q: %w", name, err)
		}
		if !allowed(sc.Tools, def.Name) {
			continue
		}
		tools = append(tools, &mcpTool{session: session, def: def, serverName: name})
	}
	return tools, nil
}

// allowed reports whether name passes list: an empty list allows every
// tool, otherwise name must appear in it exactly.
func allowed(list []string, name string) bool {
	if len(list) == 0 {
		return true
	}
	for _, n := range list {
		if n == name {
			return true
		}
	}
	return false
}
