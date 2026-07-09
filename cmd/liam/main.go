// Command liam is a minimal, opinionated coding agent harness: an
// interactive REPL, backed by GitHub Copilot Chat, that can read, write,
// and edit files and run shell commands with no confirmation prompts
// (YOLO mode). See CONTEXT.md for the project's vocabulary.
package main

import (
	"context"
	"flag"
	"os"
	"sort"

	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// systemPrompt is liam's system message: kept deliberately short, in the
// spirit of pi's sub-1000-token system prompt (see docs/references.md).
const systemPrompt = `You are liam, a minimal coding agent running directly in the user's terminal.

You have four tools: read, write, edit, and bash. Use them freely to inspect and modify the user's project and to run commands (build, test, etc.) — there is no confirmation step, so act directly rather than describing what you would do.

Prefer edit for targeted changes to existing files; use write only for new files or full rewrites. Keep responses concise.`

func main() {
	model := flag.String("model", "", "override the default Copilot model ID")
	flag.Parse()

	auth := provider.NewAuthenticator()
	p := provider.NewCopilot(auth, *model, toolDefinitions())

	runSession(context.Background(), os.Stdin, os.Stdout, os.Stderr, p, systemPrompt)
}

// toolDefinitions returns the v1 tool set's Definitions in a stable
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
