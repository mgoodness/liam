// Command liam is a minimal, opinionated coding agent harness: an
// interactive REPL, backed by GitHub Copilot Chat, that can read, write,
// and edit files and run shell commands with no confirmation prompts
// (YOLO mode). See CONTEXT.md for the project's vocabulary.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// systemPrompt is liam's system message: kept deliberately short, in the
// spirit of pi's sub-1000-token system prompt (see docs/references.md).
const systemPrompt = `You are liam, a minimal coding agent running directly in the user's terminal.

You have five tools: read, write, edit, bash, and web_fetch. Use them freely to inspect and modify the user's project, run commands (build, test, etc.), and fetch URLs — there is no confirmation step, so act directly rather than describing what you would do.

Prefer edit for targeted changes to existing files; use write only for new files or full rewrites. Keep responses concise.`

func main() {
	model := flag.String("model", "", "override the default Copilot model ID")
	flag.Parse()

	auth := provider.NewAuthenticator()
	p := provider.NewCopilot(auth, *model, toolDefinitions())

	runSession(context.Background(), os.Stdin, os.Stdout, os.Stderr, p, fullSystemPrompt(os.Stderr))
}

// fullSystemPrompt returns liam's base systemPrompt with any discovered
// AGENTS.md content appended (see loadAgentsMD). Failures resolving the
// current directory or the global AGENTS.md path are non-fatal: liam
// starts with the base prompt and a warning on errOut rather than
// refusing to run.
func fullSystemPrompt(errOut io.Writer) string {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(errOut, "warning: getwd:", err)
		return systemPrompt
	}

	agentsMD, err := loadAgentsMD(cwd, errOut)
	if err != nil {
		fmt.Fprintln(errOut, "warning: loading AGENTS.md:", err)
		return systemPrompt
	}
	if agentsMD == "" {
		return systemPrompt
	}

	return systemPrompt + "\n\n" + agentsMD
}

// toolDefinitions returns the tool set's Definitions in a stable
// (name-sorted) order, so the Provider's request body doesn't vary across
// runs due to Go's randomized map iteration.
func toolDefinitions() []tool.Definition {
	names := make([]string, 0, len(tool.Tools))
	for name := range tool.Tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]tool.Definition, len(names))
	for i, name := range names {
		defs[i] = tool.Tools[name].Definition
	}
	return defs
}
