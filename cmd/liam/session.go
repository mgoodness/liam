package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mgoodness/liam/agent"
	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// runSession drives the REPL: prompt, read a submitted message, run it
// through the agent loop against p, print the result, and repeat until the
// session ends (EOF, /exit, or /quit). tools is passed through to every
// agent.Run call, so it must be the same set whose Definitions were given
// to p. A non-auth error mid-turn is printed to errOut and the loop
// continues rather than returning.
//
// The agent loop's progress — intermediate assistant text and each tool
// call's summary and result — is printed to out via agent.Callbacks as it
// happens, in plain stdout prints with no color or redraw (see ADR 0006).
func runSession(ctx context.Context, in io.Reader, out, errOut io.Writer, p provider.Provider, tools map[string]tool.Tool, systemPrompt string) {
	r := bufio.NewReader(in)
	history := []provider.Message{{Role: provider.RoleSystem, Content: systemPrompt}}

	cb := agent.Callbacks{
		OnText: func(text string) {
			fmt.Fprintln(out, text)
		},
		OnToolCall: func(name string, args json.RawMessage) {
			fmt.Fprintln(out, "->", summarizeToolCall(tools, name, args))
		},
		OnToolResult: func(name, result string, err error) {
			fmt.Fprintln(out, result)
		},
	}

	for {
		fmt.Fprint(out, "> ")

		msg, quit, err := nextMessage(r)
		if err != nil {
			fmt.Fprintln(errOut, "error reading input:", err)
			return
		}
		if quit {
			return
		}

		history = append(history, provider.Message{Role: provider.RoleUser, Content: msg})

		_, updated, err := agent.Run(ctx, p, tools, history, cb)
		history = updated
		if err != nil {
			fmt.Fprintln(errOut, "error:", err)
			continue
		}
	}
}

// summarizeToolCall renders a one-line, human-readable description of a
// tool call for progress reporting, via that tool's own Summarize func if
// it has one, falling back to just the tool's name. tools is the same
// session-assembled set passed to agent.Run — not the package-level
// tool.Tools var, which wouldn't include a conditionally-present tool like
// web_search. Colocating the actual summaries with each tool (see
// tool.Tool.Summarize) keeps this REPL-side fallback generic rather than
// teaching it about every tool by name.
func summarizeToolCall(tools map[string]tool.Tool, name string, args json.RawMessage) string {
	t, ok := tools[name]
	if !ok || t.Summarize == nil {
		return name
	}
	if summary := t.Summarize(args); summary != "" {
		return fmt.Sprintf("%s: %s", name, summary)
	}
	return name
}
