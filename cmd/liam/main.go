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
	"github.com/mgoodness/liam/internal/skill"
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
	skillName := fs.String("skill", "", "force-activate a skill by name before the prompt (headless mode only, requires -p)")
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

	skills, err := discoverSkills(cwd, cfg, *prompt == "", stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "liam: %v\n", err)
		return 1
	}

	// -skill force-activates a skill directly, bypassing model judgment —
	// the underlying mechanism the TUI's future /skill slash command will
	// also use (ticket 16/17). It's headless-only for now: there's no
	// interactive surface to trigger it from yet.
	var forceActivated string
	if *skillName != "" {
		if *prompt == "" {
			fmt.Fprintln(stderr, "liam: -skill requires -p (headless mode)")
			return 2
		}
		s, found := skill.Find(skills, *skillName)
		if !found {
			fmt.Fprintf(stderr, "liam: unknown skill %q\n", *skillName)
			return 1
		}
		forceActivated = s.Body
	}

	p := openrouter.New(apiKey)
	tools := []tool.Tool{tool.Read{}, tool.Write{}, tool.Edit{}, tool.Bash{}}
	if catalog := skill.ModelCatalog(skills); len(catalog) > 0 {
		tools = append(tools, tool.ActivateSkill{Catalog: catalog})
	}
	loop := agent.Loop{
		Provider: p,
		Tools:    tool.NewRegistry(tools...),
	}

	if *prompt == "" {
		return runInteractive(loop, cfg, stdin, stdout, stderr)
	}
	return runHeadless(loop, cfg, *prompt, forceActivated, stdout, stderr)
}

// discoverSkills builds liam's skill catalog for this run: user-scope
// directories and skills.paths are scanned unconditionally, project-scope
// directories only if trusted. Trust is gated on a one-time per-project
// prompt in interactive mode; in headless mode (interactive is false),
// blocking on stdin mid-script isn't appropriate, so it's gated on
// cfg.Skills.TrustProjectSkills instead, defaulting to untrusted. Every
// Diagnostic Discover returns is logged to stderr, one line each.
func discoverSkills(cwd string, cfg config.Config, interactive bool, stdin io.Reader, stdout, stderr io.Writer) ([]skill.Skill, error) {
	projectTrusted := false
	if skill.HasProjectSkills(cwd) {
		// skill.OpenTrustStore and skill.ResolveProjectTrust already
		// prefix their own errors ("skill: ..."), so they're returned
		// unwrapped here rather than wrapped again.
		store, err := skill.OpenTrustStore()
		if err != nil {
			return nil, err
		}

		var promptFn skill.Prompter
		if interactive {
			promptFn = skill.TerminalPrompter(stdin, stdout)
		}

		root := skill.ProjectRoot(cwd)
		trusted, err := skill.ResolveProjectTrust(store, root, cfg.Skills.TrustProjectSkills, promptFn)
		if err != nil {
			return nil, err
		}
		projectTrusted = trusted
	}

	skills, diags := skill.Discover(skill.Options{
		Cwd:            cwd,
		ExtraPaths:     cfg.Skills.Paths,
		Disabled:       cfg.Skills.Disabled,
		ProjectTrusted: projectTrusted,
	})
	for _, d := range diags {
		fmt.Fprintf(stderr, "liam: skill: %s: %s\n", d.Path, d.Message)
	}
	return skills, nil
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
// forceActivatedSkill, when non-empty, is a force-activated skill's body
// (via -skill), carried as the turn's SystemPrompt.
func runHeadless(loop agent.Loop, cfg config.Config, prompt, forceActivatedSkill string, stdout, stderr io.Writer) int {
	req := buildRequest(cfg, prompt, forceActivatedSkill)

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
// systemPrompt, when non-empty, becomes the request's SystemPrompt — used
// by -skill's force-activation to inject a skill's body ahead of prompt.
func buildRequest(cfg config.Config, prompt, systemPrompt string) provider.Request {
	return provider.Request{
		Model:        cfg.Provider.Model,
		SystemPrompt: systemPrompt,
		Messages:     []provider.Message{{Role: "user", Content: prompt}},
	}
}
