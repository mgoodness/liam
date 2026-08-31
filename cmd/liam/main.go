// Command liam is a minimal Go coding-agent harness.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/provider/openrouter"
	"github.com/mgoodness/liam/internal/render"
	"github.com/mgoodness/liam/internal/tool"
	"github.com/mgoodness/liam/internal/tui"
)

// version is set via ldflags (-X main.version=...) by GoReleaser. Plain
// `go install` builds leave it at "dev", so versionString falls back to the
// module version the Go toolchain embeds via debug.ReadBuildInfo.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("liam", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prompt := fs.String("p", "", "send a single prompt headlessly and exit")
	model := fs.String("model", "", "override the provider.model config value")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, versionString())
		return 0
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(stderr, "liam: OPENROUTER_API_KEY is not set")
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "liam: %v\n", err)
		return 1
	}
	cfg, err := config.Load(cwd, *model)
	if err != nil {
		fmt.Fprintf(stderr, "liam: %v\n", err)
		return 1
	}

	p := openrouter.New(apiKey)
	loop := agent.Loop{
		Provider: p,
		Tools:    tool.NewRegistry(tool.Read{}, tool.Write{}, tool.Edit{}, tool.Bash{}),
	}

	if *prompt == "" {
		return runInteractive(loop, cfg, stdin, stdout, stderr)
	}
	return runHeadless(loop, cfg, *prompt, stdout, stderr)
}

// runInteractive launches liam's Bubbletea TUI.
func runInteractive(loop agent.Loop, cfg config.Config, stdin io.Reader, stdout, stderr io.Writer) int {
	m := tui.New(loop, cfg)
	opts := []tea.ProgramOption{tea.WithInput(stdin), tea.WithOutput(stdout)}
	p := tea.NewProgram(m, opts...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(stderr, "liam: %v\n", err)
		return 1
	}
	return 0
}

// runHeadless sends prompt through loop as a single turn, streaming
// assistant text and tool-call/result lines to stdout as they arrive, and
// noting the actually-used model to stderr once per response.
func runHeadless(loop agent.Loop, cfg config.Config, prompt string, stdout, stderr io.Writer) int {
	req := buildRequest(cfg, prompt)

	var wroteText bool
	_, err := loop.Run(context.Background(), req, func(ev provider.Event) {
		switch e := ev.(type) {
		case provider.TextDeltaEvent:
			fmt.Fprint(stdout, e.Text)
			wroteText = true
		case provider.ToolResultEvent:
			if wroteText {
				fmt.Fprintln(stdout)
				wroteText = false
			}
			fmt.Fprintf(stdout, "⚙ %s\n", render.ToolCall(e.Name, e.ArgsJSON, e.Content, e.IsError))
		case provider.DoneEvent:
			// One model= note per response (turn), per spec: auto-routing can
			// pick a different model turn to turn. A tool-calls-only turn (no
			// streamed text) skips the trailing blank line.
			if wroteText {
				fmt.Fprintln(stdout)
				wroteText = false
			}
			fmt.Fprintf(stderr, "liam: model=%s\n", e.ModelUsed)
		}
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, "liam: [interrupted]")
		} else {
			fmt.Fprintf(stderr, "liam: %v\n", err)
		}
		return 1
	}

	return 0
}

// buildRequest assembles the provider.Request for a headless prompt,
// threading cfg.Provider.Model through as the model actually sent to the
// provider (empty leaves the provider's own default in place).
func buildRequest(cfg config.Config, prompt string) provider.Request {
	return provider.Request{
		Model:    cfg.Provider.Model,
		Messages: []provider.Message{{Role: "user", Content: prompt}},
	}
}
