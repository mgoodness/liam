package main

import (
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/input"

	"github.com/mgoodness/liam/skill"
)

// eventSource pulls one input event at a time out of rd. A single
// underlying read can produce events for more than one message (e.g. two
// lines typed, or pasted, faster than nextMessage drains them); eventSource
// buffers whatever's left over after a batch is only partially consumed, so
// the next call to next picks up exactly where the previous one left off
// instead of silently dropping already-parsed events.
type eventSource struct {
	rd      *input.Reader
	pending []input.Event
}

func newEventSource(rd *input.Reader) *eventSource {
	return &eventSource{rd: rd}
}

func (s *eventSource) next() (input.Event, error) {
	for len(s.pending) == 0 {
		events, err := s.rd.ReadEvents()
		if err != nil {
			return nil, err
		}
		s.pending = events
	}
	ev := s.pending[0]
	s.pending = s.pending[1:]
	return ev, nil
}

// crlf is what nextMessage and writeLine (session.go) both write in place
// of a bare "\n": raw mode disables the terminal driver's own translation
// from "\n" to a proper carriage-return-then-newline (see terminal.go), so
// every line ending has to spell out the "\r" itself.
const crlf = "\r\n"

// nextMessage reads structured input events from src — key presses,
// including Kitty keyboard protocol modifiers, and bracketed-paste
// content — accumulating them into a single message. It returns quit=true,
// with no message, when the session should end: Ctrl+C, Ctrl+D, a literal
// /exit or /quit message, or src reaching EOF. A genuine read error
// (anything but io.EOF) is returned as-is rather than treated as a clean
// session end.
//
// Plain Enter submits the accumulated message immediately — no second
// blank-line Enter required, replacing v0.1's blank-line-terminated
// submission entirely. Shift+Enter and the universal Ctrl+J fallback
// insert a newline without submitting, for manual multi-line composition.
// A bracketed-paste event submits immediately too, folding in whatever was
// already typed and regardless of whether the pasted text itself ends in a
// newline — this is what resolves the ambiguity a bufio.Reader.Buffered()
// heuristic would have had (see ADR 0002).
//
// echo receives a minimal transcript of what was typed or pasted — raw
// text, newlines, and a backspace-erase sequence — so the caller can make
// raw-mode input visible on the terminal; nextMessage performs no terminal
// rendering itself.
func nextMessage(src *eventSource, echo io.Writer) (msg string, quit bool, err error) {
	var lines []string
	var cur strings.Builder

	newline := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		io.WriteString(echo, crlf)
	}

	submit := func() string {
		newline()
		full := strings.Join(lines, "\n")
		lines = nil
		return full
	}

	for {
		ev, readErr := src.next()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return "", true, nil
			}
			return "", false, readErr
		}

		if paste, ok := ev.(input.PasteEvent); ok {
			content := string(paste)
			cur.WriteString(content)
			io.WriteString(echo, content)
			return submit(), false, nil
		}

		key, isKey := ev.(input.KeyPressEvent)
		if !isKey {
			continue
		}

		switch {
		case key.Code == input.KeyEnter && key.Mod.Contains(input.ModShift),
			key.Mod.Contains(input.ModCtrl) && key.Code == 'j':
			newline()

		case key.Code == input.KeyEnter:
			switch full := submit(); strings.TrimSpace(full) {
			case "":
				continue
			case "/exit", "/quit":
				return "", true, nil
			default:
				return full, false, nil
			}

		case key.Mod.Contains(input.ModCtrl) && (key.Code == 'c' || key.Code == 'd'):
			io.WriteString(echo, crlf)
			return "", true, nil

		case key.Code == input.KeyBackspace:
			s := cur.String()
			if s == "" {
				continue
			}
			_, size := utf8.DecodeLastRuneInString(s)
			cur.Reset()
			cur.WriteString(s[:len(s)-size])
			io.WriteString(echo, "\b \b")

		case key.Text != "":
			cur.WriteString(key.Text)
			io.WriteString(echo, key.Text)
		}
	}
}

// resolveSkillCommand parses msg as an explicit /name invocation (optionally
// followed by trailing text) and resolves name against skills — the full
// discovered set from skill.Discover, not the model-facing index skill.Index
// renders, so a disable-model-invocation skill remains resolvable here even
// though it's excluded from the system prompt (see ADR 0003). The name ends
// at the first run of whitespace — including a newline, since nextMessage
// lets a submitted message span multiple lines (Shift+Enter, Ctrl+J) — and
// everything after it is trailing text, returned unchanged with no
// positional-argument substitution.
//
// ok is false when msg doesn't start with "/" or names no discovered skill;
// the caller should then treat msg as an ordinary message rather than a
// skill invocation, which is what keeps an unmatched name from crashing the
// session.
func resolveSkillCommand(skills []skill.Skill, msg string) (matched skill.Skill, text string, ok bool) {
	rest, isSlash := strings.CutPrefix(msg, "/")
	if !isSlash {
		return skill.Skill{}, "", false
	}

	name := rest
	var trailing string
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		name, trailing = rest[:i], rest[i+1:]
	}

	for _, s := range skills {
		if s.Name == name {
			return s, strings.TrimSpace(trailing), true
		}
	}
	return skill.Skill{}, "", false
}
