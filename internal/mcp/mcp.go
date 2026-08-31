// Package mcp is liam's MCP client: it connects to servers configured
// under mcpServers over stdio, and registers each one's tools capability
// into liam's toolset as ordinary tool.Tools.
package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/tool"
)

// DefaultLoadTimeout bounds how long a caller waits on (*Loader).Tools
// before proceeding with whatever's loaded so far (issue #48).
const DefaultLoadTimeout = 5 * time.Second

// ToolLoader is the subset of *Loader's API a caller needs to wait for and
// merge loaded tools — narrow enough for tests to substitute a fake
// without a real Loader, and shared by every caller (rather than each
// defining its own identically-shaped interface) since they already
// depend on this package concretely for Start/DefaultLoadTimeout.
type ToolLoader interface {
	Tools(ctx context.Context, timeout time.Duration) (tools []tool.Tool, timedOut bool)
	Errs() map[string]error
}

// Merge waits for loader (bounded by DefaultLoadTimeout, or by ctx —
// e.g. Escape-cancellation — whichever comes first), merges whatever
// tools loaded into registry, and reports a timeout or any per-server
// load failure via warn. A nil loader (no mcpServers configured) is a
// no-op. Shared by liam's headless and interactive entry points so the
// wait/merge/warn sequence isn't duplicated per caller.
func Merge(ctx context.Context, registry tool.Registry, loader ToolLoader, warn func(msg string)) {
	if loader == nil {
		return
	}
	tools, timedOut := loader.Tools(ctx, DefaultLoadTimeout)
	if timedOut {
		warn(fmt.Sprintf("mcp: load timed out after %s, proceeding with whatever loaded", DefaultLoadTimeout))
	}
	for name, err := range loader.Errs() {
		warn(fmt.Sprintf("mcp: %s: %v", name, err))
	}
	for _, t := range tools {
		registry[t.Name()] = t
	}
}

// Loader connects to every configured MCP server in the background,
// starting immediately when constructed via Start. Call (*Loader).Tools to
// wait for loading (bounded by a timeout) and retrieve whatever tools are
// ready by then.
type Loader struct {
	done chan struct{}

	mu    sync.Mutex
	tools []tool.Tool
	errs  map[string]error
}

// Start begins loading every server in servers in the background, each
// launched as a real stdio subprocess, and returns immediately — liam
// stays usable with its built-in tools while this runs.
func Start(ctx context.Context, servers config.MCPServersConfig) *Loader {
	return start(ctx, servers, func(_ string, sc config.MCPServerConfig) sdkmcp.Transport {
		return newCommandTransport(sc)
	})
}

// start is Start's test seam: newTransport lets a test substitute an
// in-memory transport per server instead of spawning a real subprocess.
func start(ctx context.Context, servers config.MCPServersConfig, newTransport func(name string, sc config.MCPServerConfig) sdkmcp.Transport) *Loader {
	l := &Loader{done: make(chan struct{}), errs: map[string]error{}}
	go l.loadAll(ctx, servers, newTransport)
	return l
}

func (l *Loader) loadAll(ctx context.Context, servers config.MCPServersConfig, newTransport func(string, config.MCPServerConfig) sdkmcp.Transport) {
	defer close(l.done)

	var wg sync.WaitGroup
	for name, sc := range servers {
		wg.Add(1)
		go func(name string, sc config.MCPServerConfig) {
			defer wg.Done()
			tools, err := loadServer(ctx, name, sc, func(sc config.MCPServerConfig) sdkmcp.Transport {
				return newTransport(name, sc)
			})

			l.mu.Lock()
			defer l.mu.Unlock()
			if err != nil {
				l.errs[name] = err
				return
			}
			l.tools = append(l.tools, tools...)
		}(name, sc)
	}
	wg.Wait()
}

// Tools blocks until every configured server has finished loading, ctx is
// done (e.g. Escape-cancellation), or timeout elapses — whichever comes
// first — then returns whatever tools were ready by that point. timedOut
// is set only when the timeout itself fired, not on ctx cancellation
// (callers already treat a canceled ctx as its own outcome, e.g.
// agent.Loop.Run's usual context.Canceled handling — this isn't a second,
// conflicting reason to warn). Safe to call more than once; a call after
// loading has already finished returns immediately.
func (l *Loader) Tools(ctx context.Context, timeout time.Duration) (tools []tool.Tool, timedOut bool) {
	select {
	case <-l.done:
	case <-ctx.Done():
	case <-time.After(timeout):
		timedOut = true
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]tool.Tool(nil), l.tools...), timedOut
}

// Errs returns each server's own load error (a connect, handshake, or
// list-tools failure), keyed by server name. Safe to call any time;
// reflects whatever has completed so far.
func (l *Loader) Errs() map[string]error {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]error, len(l.errs))
	for k, v := range l.errs {
		out[k] = v
	}
	return out
}
