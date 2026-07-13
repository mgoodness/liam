package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/input"

	"github.com/mgoodness/liam/agent"
	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/skill"
	"github.com/mgoodness/liam/tool"
)

// runSession drives the REPL: prompt, read a submitted message, run it
// through the agent loop against p, print the result, and repeat until the
// session ends (EOF, /exit, or /quit). tools is passed through to every
// agent.Run call, so it must be the same set whose Definitions were given
// to p. A non-auth error mid-turn is printed to errOut and the loop
// continues rather than returning.
//
// Each turn gets its own cancelable context, derived fresh from ctx rather
// than reusing one long-lived context across every turn, so cancelling one
// turn can't affect the next. While agent.Run is in flight, a concurrent
// watchForCtrlC call watches src for a Ctrl+C key event and cancels that
// turn's context the instant it sees one; runSession then recognizes the
// resulting context.Canceled error and returns quietly to the prompt
// instead of printing it to errOut like a genuine failure — see ADR 0007.
// Ctrl+C between turns, while nextMessage itself is waiting at the prompt,
// is unaffected and continues to end the session exactly as before.
//
// A submitted message that resolves against skills as a /name invocation
// (see resolveSkillCommand) loads that skill's full SKILL.md body via the
// existing read tool before the turn runs, rather than being sent to the
// model as literal text — see ADR 0003. skills is the full discovered set,
// including disable-model-invocation skills, which remain explicitly
// invocable even though they're absent from systemPrompt's model-facing
// index.
//
// The agent loop's progress — intermediate assistant text and each tool
// call's summary and result — is printed to out via agent.Callbacks as it
// happens, via writeLine so it renders correctly under raw mode (see
// terminal.go and ADR 0006).
func runSession(ctx context.Context, in io.Reader, out, errOut io.Writer, p provider.Provider, tools map[string]tool.Tool, systemPrompt string, skills []skill.Skill) {
	rd, err := input.NewReader(in, "", 0)
	if err != nil {
		writeLine(errOut, "error initializing input reader:", err)
		return
	}
	defer rd.Close()
	src := newEventSource(rd)

	ch := make(chan eventOrErr)
	go pump(src, ch)
	h := &handoff{ch: ch}

	history := []provider.Message{{Role: provider.RoleSystem, Content: systemPrompt}}

	cb := agent.Callbacks{
		OnText: func(text string) {
			writeLine(out, text)
		},
		OnToolCall: func(name string, args json.RawMessage) {
			writeLine(out, "->", summarizeToolCall(tools, name, args))
		},
		OnToolResult: func(name, result string, err error) {
			writeLine(out, result)
		},
	}

	for {
		fmt.Fprint(out, "> ")

		msg, quit, err := nextMessage(h, out)
		if err != nil {
			writeLine(errOut, "error reading input:", err)
			return
		}
		if quit {
			return
		}

		if s, text, ok := resolveSkillCommand(skills, msg); ok {
			body, err := loadSkillBody(ctx, tools, s)
			if err != nil {
				writeLine(errOut, "error loading skill", s.Name+":", err)
				continue
			}
			writeLine(out, "-> skill:", s.Name)
			msg = body
			if text != "" {
				msg += "\n\n" + text
			}
		}

		history = append(history, provider.Message{Role: provider.RoleUser, Content: msg})

		turnCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		watchDone := make(chan []eventOrErr, 1)
		go func() {
			watchDone <- watchForCtrlC(ch, cancel, done)
		}()

		_, updated, runErr := agent.Run(turnCtx, p, tools, history, cb)
		close(done)
		h.replay = append(h.replay, <-watchDone...)
		cancel()

		history = updated
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) {
				continue
			}
			writeLine(errOut, "error:", runErr)
			continue
		}
	}
}

// loadSkillBody loads s's full SKILL.md body via the existing read tool
// (see ADR 0003) rather than a dedicated skill-loading mechanism, the same
// way the model itself loads a skill it discovers via the system prompt
// index.
func loadSkillBody(ctx context.Context, tools map[string]tool.Tool, s skill.Skill) (string, error) {
	args, err := json.Marshal(map[string]string{"path": s.SkillMDPath()})
	if err != nil {
		return "", err
	}
	return tool.Call(ctx, tools, "read", args)
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

// writeLine prints a to w as a line, translating every "\n" — including
// ones embedded in a multi-line assistant response — to crlf. Raw mode
// (see terminal.go) disables the terminal driver's own translation from a
// bare "\n" to a proper carriage-return-then-newline, so without this,
// each line after the first would render one column further right than
// the last instead of returning to the start of the line.
func writeLine(w io.Writer, a ...any) {
	line := strings.TrimSuffix(fmt.Sprintln(a...), "\n")
	fmt.Fprint(w, strings.ReplaceAll(line, "\n", crlf), crlf)
}
