package main

import (
	"context"
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

// eventReader is anything nextMessage can pull one input event at a time
// from — an *eventSource directly (used between turns before a turn's
// concurrent watcher exists, and throughout in tests), or a *handoff (used
// by runSession, layering the mid-turn Ctrl+C watcher on top).
type eventReader interface {
	next() (input.Event, error)
}

// eventOrErr carries one eventSource.next result — an event or a terminal
// read error — so pump can hand either off over a single channel.
type eventOrErr struct {
	ev  input.Event
	err error
}

// pump is the sole goroutine that ever calls src.next: the underlying
// input.Reader's blocking read isn't safe for concurrent use, so every
// consumer — nextMessage between turns, watchForCtrlC while a turn is in
// flight — reads from ch instead of touching src directly, and ownership of
// "who's draining ch right now" simply moves between them without either
// ever racing the actual read. pump runs for the life of the session,
// stopping once src.next returns any error (a clean EOF or a genuine read
// failure alike) — that error is sent to ch like any other result, so
// whichever consumer is reading at the time still sees it rather than
// hanging forever waiting for input that will never come.
func pump(src *eventSource, ch chan<- eventOrErr) {
	for {
		ev, err := src.next()
		ch <- eventOrErr{ev, err}
		if err != nil {
			return
		}
	}
}

// handoff is nextMessage's view of pump's channel between turns: it first
// replays, in order, whatever a mid-turn watchForCtrlC call read off ch but
// didn't act on — keystrokes typed while a turn was in flight, or even the
// terminal error that ends the session — before pulling anything fresh off
// ch itself. This is what keeps an event from being silently dropped at the
// boundary between a turn ending and the next nextMessage call beginning.
type handoff struct {
	ch     <-chan eventOrErr
	replay []eventOrErr
}

func (h *handoff) next() (input.Event, error) {
	if len(h.replay) > 0 {
		item := h.replay[0]
		h.replay = h.replay[1:]
		return item.ev, item.err
	}
	item := <-h.ch
	return item.ev, item.err
}

// isCtrlC reports whether key is the Ctrl+C combination — the one event
// both nextMessage (ending the session when idle at the prompt) and
// watchForCtrlC (cancelling the current turn's context when one's in
// flight) need to recognize identically.
func isCtrlC(key input.KeyPressEvent) bool {
	return key.Mod.Contains(input.ModCtrl) && key.Code == 'c'
}

// watchForCtrlC reads events off ch while a turn is in flight, watching
// specifically for Ctrl+C, and calls cancel the instant it sees one before
// returning. It also returns as soon as done is closed, which runSession
// does the moment the turn ends on its own (a normal response or a
// mid-turn error) — whichever happens first, ctrl+c or done, no event still
// left on ch is read after that point by this call.
//
// Any event read that isn't Ctrl+C — including the terminal error pump
// sends when input ends — is appended to leftover in arrival order rather
// than discarded, so runSession can splice it into the handoff for the next
// nextMessage call to replay.
func watchForCtrlC(ch <-chan eventOrErr, cancel context.CancelFunc, done <-chan struct{}) (leftover []eventOrErr) {
	for {
		select {
		case <-done:
			return leftover
		case item := <-ch:
			if item.err == nil {
				if key, ok := item.ev.(input.KeyPressEvent); ok && isCtrlC(key) {
					cancel()
					return leftover
				}
			}
			leftover = append(leftover, item)
			if item.err != nil {
				return leftover
			}
		}
	}
}

// crlf is what nextMessage and writeLine (session.go) both write in place
// of a bare "\n": raw mode disables the terminal driver's own translation
// from "\n" to a proper carriage-return-then-newline (see terminal.go), so
// every line ending has to spell out the "\r" itself.
const crlf = "\r\n"

// nextMessage reads structured input events from src — key presses,
// including Kitty keyboard protocol modifiers, and bracketed-paste
// content — accumulating them into a single message. It returns quit=true,
// with no message, when the session should end: Ctrl+C, Ctrl+D on an empty
// buffer, a literal /exit or /quit message, or src reaching EOF. A genuine
// read error (anything but io.EOF) is returned as-is rather than treated as
// a clean session end.
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
// Ctrl+D matches canonical-mode/bash/Python-REPL convention (ADR 0007): it
// only ends the session on an empty buffer. With a non-empty buffer —
// typed characters or lines accumulated via Shift+Enter/Ctrl+J — it
// submits instead, following the same path as Enter. Ctrl+C always ends
// the session, abandoning whatever's been typed.
//
// echo receives a minimal transcript of what was typed or pasted — raw
// text, newlines, and a backspace-erase sequence — so the caller can make
// raw-mode input visible on the terminal; nextMessage performs no terminal
// rendering itself.
func nextMessage(src eventReader, echo io.Writer) (msg string, quit bool, err error) {
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

	// trySubmit runs the same submission logic as plain Enter: a blank
	// accumulated buffer is ignored (the loop keeps reading), /exit and
	// /quit end the session, and anything else is returned as the
	// submitted message. done reports whether the caller should return
	// immediately with (msg, quit, nil) or keep looping.
	trySubmit := func() (msg string, quit bool, done bool) {
		switch full := submit(); strings.TrimSpace(full) {
		case "":
			return "", false, false
		case "/exit", "/quit":
			return "", true, true
		default:
			return full, false, true
		}
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
			if msg, quit, done := trySubmit(); done {
				return msg, quit, nil
			}

		case key.Mod.Contains(input.ModCtrl) && key.Code == 'd':
			// Ctrl+D only ends the session on an empty buffer, matching
			// canonical-mode/bash/Python-REPL convention (ADR 0007). With
			// something typed, it submits exactly like Enter instead.
			if cur.Len() == 0 && len(lines) == 0 {
				io.WriteString(echo, crlf)
				return "", true, nil
			}
			if msg, quit, done := trySubmit(); done {
				return msg, quit, nil
			}

		case isCtrlC(key):
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
