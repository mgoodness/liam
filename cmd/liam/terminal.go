package main

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// enableTUIInput negotiates the terminal features nextMessage's input
// layer depends on: raw mode, so Kitty keyboard and bracketed-paste escape
// sequences reach us as they arrive instead of being buffered line-by-line
// by the tty driver, and the Kitty keyboard protocol's
// disambiguate-escape-codes flag, so Shift+Enter can be told apart from
// plain Enter (see ADR 0005). It's a no-op, returning a no-op restore,
// when in isn't an interactive terminal (e.g. piped input).
//
// This is raw-mode toggling glue, deliberately untested — like main.go's
// existing wiring — per issue #19's acceptance criteria.
func enableTUIInput(in *os.File, out io.Writer) (restore func()) {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return func() {}
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}
	}

	fmt.Fprint(out, ansi.PushKittyKeyboard(ansi.KittyDisambiguateEscapeCodes))
	fmt.Fprint(out, ansi.SetModeBracketedPaste)

	return func() {
		fmt.Fprint(out, ansi.ResetModeBracketedPaste)
		fmt.Fprint(out, ansi.PopKittyKeyboard(1))
		_ = term.Restore(fd, state)
	}
}
