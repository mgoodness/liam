package main

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/mgoodness/liam/agent"
	"github.com/mgoodness/liam/provider"
)

// runSession drives the REPL: prompt, read a submitted message, run it
// through the agent loop against p, print the result, and repeat until the
// session ends (EOF, /exit, or /quit). A non-auth error mid-turn is printed
// to errOut and the loop continues rather than returning.
func runSession(ctx context.Context, in io.Reader, out, errOut io.Writer, p provider.Provider, systemPrompt string) {
	r := bufio.NewReader(in)
	history := []provider.Message{{Role: provider.RoleSystem, Content: systemPrompt}}

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

		text, updated, err := agent.Run(ctx, p, history)
		history = updated
		if err != nil {
			fmt.Fprintln(errOut, "error:", err)
			continue
		}

		fmt.Fprintln(out, text)
	}
}
