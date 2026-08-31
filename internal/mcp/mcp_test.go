package mcp

import (
	"context"
	"os/exec"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mgoodness/liam/internal/config"
)

func TestStartReturnsImmediately(t *testing.T) {
	// hungClientTransport's other half is never connected to a server, so
	// loading it would hang forever if start() blocked on it directly.
	hungClientTransport, _ := sdkmcp.NewInMemoryTransports()
	newTransport := func(string, config.MCPServerConfig) sdkmcp.Transport { return hungClientTransport }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		start(ctx, config.MCPServersConfig{"slow": {}}, newTransport)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("start() did not return promptly; want loading to happen in the background")
	}
}

func TestLoaderToolsWaitsForCompletionWithoutTimingOut(t *testing.T) {
	newTransport := func(string, config.MCPServerConfig) sdkmcp.Transport {
		return stubTransport(t, func(s *sdkmcp.Server) {
			sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "greet", Description: "say hi"}, greetHandler)
		})
	}

	l := start(context.Background(), config.MCPServersConfig{"fast": {}}, newTransport)

	tools, timedOut := l.Tools(context.Background(), 2*time.Second)
	if timedOut {
		t.Errorf("timedOut = true, want false (server loads well within the timeout)")
	}
	if len(tools) != 1 || tools[0].Name() != "greet" {
		t.Fatalf("tools = %+v, want [greet]", tools)
	}
}

// TestLoaderToolsReturnsPartialResultOnTimeout covers issue #48's core
// async-load contract: a server that never responds must not block Tools
// past its timeout, and tools from OTHER servers that did finish in time
// must still come back.
func TestLoaderToolsReturnsPartialResultOnTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hungClientTransport, _ := sdkmcp.NewInMemoryTransports()
	newTransport := func(name string, _ config.MCPServerConfig) sdkmcp.Transport {
		if name == "slow" {
			return hungClientTransport
		}
		return stubTransport(t, func(s *sdkmcp.Server) {
			sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "greet", Description: "say hi"}, greetHandler)
		})
	}

	l := start(ctx, config.MCPServersConfig{"fast": {}, "slow": {}}, newTransport)

	tools, timedOut := l.Tools(context.Background(), 200*time.Millisecond)
	if !timedOut {
		t.Errorf("timedOut = false, want true (the slow server never responds)")
	}
	if len(tools) != 1 || tools[0].Name() != "greet" {
		t.Fatalf("tools = %+v, want only [greet] from the fast server", tools)
	}
}

// TestLoaderToolsReturnsWhenContextCanceled covers Escape-cancellation
// during the wait itself: a caller's ctx being canceled must interrupt
// Tools promptly, independent of (and well before) the timeout — mirroring
// how a Provider.Stream or Tool.Run call already respects ctx.
func TestLoaderToolsReturnsWhenContextCanceled(t *testing.T) {
	hungClientTransport, _ := sdkmcp.NewInMemoryTransports()
	newTransport := func(string, config.MCPServerConfig) sdkmcp.Transport { return hungClientTransport }

	loaderCtx, loaderCancel := context.WithCancel(context.Background())
	defer loaderCancel()
	l := start(loaderCtx, config.MCPServersConfig{"slow": {}}, newTransport)

	callerCtx, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()

	done := make(chan struct{})
	var timedOut bool
	go func() {
		_, timedOut = l.Tools(callerCtx, 10*time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Tools() did not return promptly on a canceled context")
	}
	if timedOut {
		t.Error("timedOut = true, want false (canceled, not timed out — a distinct outcome)")
	}
}

func TestLoaderErrsSurfacesPerServerConnectFailure(t *testing.T) {
	newTransport := func(string, config.MCPServerConfig) sdkmcp.Transport {
		return &sdkmcp.CommandTransport{Command: exec.Command("/no/such/binary-liam-test")}
	}

	l := start(context.Background(), config.MCPServersConfig{"bad": {}}, newTransport)
	l.Tools(context.Background(), 2*time.Second) // wait for the (fast) connect failure to be recorded

	errs := l.Errs()
	if errs["bad"] == nil {
		t.Errorf("Errs()[\"bad\"] = nil, want a connect error")
	}
}
