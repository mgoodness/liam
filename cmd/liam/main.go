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
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/instructions"
	"github.com/mgoodness/liam/internal/mcp"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/provider/openrouter"
	"github.com/mgoodness/liam/internal/render"
	"github.com/mgoodness/liam/internal/session"
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

	// Issue #56: every AGENTS.md/LIAM.md found walking from the git root
	// (or cwd) down to cwd, plus the personal $XDG_CONFIG_HOME/liam/LIAM.md,
	// concatenated general-to-specific.
	projectInstructions, err := instructions.Load(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "liam: %v\n", err)
		return 1
	}

	// Issue #95: liam's fixed identity preamble goes first, ahead of issue
	// #56's discovered project instructions — the base of every turn's
	// SystemPrompt, before any headless -skill force-activation is layered
	// on top.
	systemPrompt := baseSystemPrompt(projectInstructions)

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
	findSearcher, grepSearcher := findGrepSearchers(cwd, stderr)
	tools := []tool.Tool{tool.Read{}, tool.Write{}, tool.Edit{}, tool.Bash{}, tool.Find{Searcher: findSearcher}, tool.Grep{Searcher: grepSearcher}, tool.WebFetch{}}
	// web_search is silently unregistered when EXA_API_KEY is unset
	// (issue #50's spec), rather than erroring like OPENROUTER_API_KEY's
	// absence above — Exa is an optional dependency, not liam's core
	// model-calling path.
	if exaKey := os.Getenv("EXA_API_KEY"); exaKey != "" {
		tools = append(tools, tool.WebSearch{APIKey: exaKey})
	}
	if catalog := skill.ModelCatalog(skills); len(catalog) > 0 {
		tools = append(tools, tool.ActivateSkill{Catalog: catalog})
	}
	hooks := &hook.Runner{
		Hooks: cfg.Hooks,
		Cwd:   cwd,
		Warn:  func(msg string) { fmt.Fprintf(stderr, "liam: %s\n", msg) },
	}
	loop := agent.Loop{
		Provider: p,
		Tools:    tool.NewRegistry(tools...),
		Hooks:    hooks,
	}

	// MCP tool loading starts now, in the background — liam stays usable
	// with built-in tools immediately; the first actual model call blocks
	// on this (bounded by mcp.DefaultLoadTimeout) via mergeMCPTools.
	mcpLoader := mcp.Start(context.Background(), cfg.MCPServers)

	if *prompt == "" {
		deps := interactiveDeps{
			loop:         loop,
			mcpLoader:    mcpLoader,
			cfg:          cfg,
			skills:       skills,
			systemPrompt: systemPrompt,
			findSearcher: findSearcher,
			cwd:          cwd,
		}
		return runInteractive(deps, stdin, stdout, stderr)
	}
	return runHeadless(loop, mcpLoader, cfg, *prompt, joinPrompt(systemPrompt, forceActivated), stdout, stderr)
}

// baseSystemPrompt combines liam's fixed identity preamble (issue #95) with
// projectInstructions — issue #56's discovered AGENTS.md/LIAM.md content, or
// "" if none was found — preamble first, general-to-specific, matching
// joinPrompt's own separator convention. The preamble is always present,
// even when projectInstructions is empty, and is composed here, outside
// instructions.Load() entirely, so Load's per-file/total size caps (which
// apply only to discovered files) can never truncate it.
func baseSystemPrompt(projectInstructions string) string {
	return joinPrompt(instructions.Preamble, projectInstructions)
}

// joinPrompt joins two general-to-specific SystemPrompt layers with a blank
// line — used both by baseSystemPrompt (identity preamble, then issue #56's
// discovered AGENTS.md/LIAM.md instructions) and by run() itself (that
// combined base, then a -skill force-activated body, specific to one
// headless invocation). Either side may be empty; joinPrompt skips it
// rather than leaving a stray separator.
func joinPrompt(project, skill string) string {
	switch {
	case project == "":
		return skill
	case skill == "":
		return project
	default:
		return project + "\n\n" + skill
	}
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

// findGrepSearchers backs find/grep with tool.StdlibSearch rooted at cwd,
// unconditionally — the sole searcher since issue #97 removed the
// hardwired fff-mcp special-case (ticket #49's auto-detect-on-$PATH,
// internal, non-user-visible MCP connection). A native (non-MCP) fff
// integration is tracked separately for v2+ — see
// docs/research/golang-fff-alternatives.md. The active searcher is still
// logged to stderr, matching ticket #18's resolution.
func findGrepSearchers(cwd string, stderr io.Writer) (tool.FindSearcher, tool.GrepSearcher) {
	fmt.Fprintln(stderr, "liam: find/grep searcher=stdlib")
	stdlib := tool.StdlibSearch{Dir: cwd}
	return stdlib, stdlib
}

// interactiveDeps bundles runInteractive's TUI-construction dependencies,
// kept separate from stdin/stdout/stderr (still plain io params, matching
// runHeadless's own convention) so the two don't blur into one long,
// same-shaped parameter list.
type interactiveDeps struct {
	loop         agent.Loop
	mcpLoader    mcp.ToolLoader
	cfg          config.Config
	skills       []skill.Skill
	systemPrompt string // issue #95's identity preamble + issue #56's discovered AGENTS.md/LIAM.md instructions (baseSystemPrompt)
	findSearcher tool.FindSearcher
	cwd          string // issue #60's statusLine "cwd" field and built-in git info
}

// runInteractive launches liam's Bubbletea TUI. deps.loop.Hooks' sessionEnd,
// if set, is guaranteed to fire exactly once when p.Run() returns — a
// single structural guarantee (matching runHeadless's own defer) rather
// than one hand-threaded into every quit path inside the TUI itself, since
// deps.loop.Hooks is the same *hook.Runner the TUI's own New/handleKey/
// submit share and keep pointed at whatever session is current (including
// across /clear's session swap). deps.mcpLoader is attached to the Model,
// waited on (bounded by a timeout) and merged into the toolset on the
// session's first turn only — see tui.Model.WithMCPLoader.
// deps.systemPrompt is carried on every turn via tui.Model.WithSystemPrompt.
// deps.findSearcher backs the "@"-file-reference autocomplete popup (issue
// #58) via tui.Model.WithFindSearcher — the same searcher findGrepSearchers
// picked for the find/grep tools themselves. deps.cwd feeds statusLine's
// (issue #60) "cwd" field and the built-in renderer's git branch/dirty
// lookup via tui.Model.WithCwd.
func runInteractive(deps interactiveDeps, stdin io.Reader, stdout, stderr io.Writer) int {
	m := tui.New(deps.loop, deps.cfg, deps.skills).
		WithMCPLoader(deps.mcpLoader).
		WithSystemPrompt(deps.systemPrompt).
		WithFindSearcher(deps.findSearcher).
		WithCwd(deps.cwd)
	if deps.loop.Hooks != nil {
		defer deps.loop.Hooks.SessionEnd(context.Background())
	}
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
// noting the actually-used model to stderr once per response. systemPrompt
// becomes the turn's SystemPrompt as-is — the caller (run) has already
// combined issue #95's identity preamble and issue #56's discovered project
// instructions (baseSystemPrompt) with any -skill force-activated body via
// joinPrompt. mcpLoader's tools are waited for
// and merged into loop.Tools before the turn runs — headless mode has
// exactly one turn, so this trivially satisfies "the first actual model
// call blocks on MCP load completion."
func runHeadless(loop agent.Loop, mcpLoader mcp.ToolLoader, cfg config.Config, prompt, systemPrompt string, stdout, stderr io.Writer) int {
	req := buildRequest(cfg, prompt, systemPrompt)

	ctx := context.Background()
	mcp.Merge(ctx, loop.Tools, mcpLoader, func(msg string) { fmt.Fprintf(stderr, "liam: %s\n", msg) })

	if loop.Hooks != nil {
		loop.Hooks.SessionID = session.New().ID
		loop.Hooks.SessionStart(ctx)
		defer loop.Hooks.SessionEnd(ctx)
	}

	var wroteText bool
	_, err := loop.Run(ctx, req, func(ev provider.Event) {
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
