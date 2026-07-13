package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/x/input"

	"github.com/mgoodness/liam/skill"
)

// chunkedReader delivers a fixed sequence of byte chunks, one per Read
// call, simulating how bytes actually arrive from a terminal: each key
// press or paste event tends to show up as its own read, rather than the
// whole session arriving in one shot. It never splits a single chunk
// across two Read calls, so a chunk must be a complete, well-formed escape
// sequence or a single input event's raw bytes end-to-end.
type chunkedReader struct {
	chunks [][]byte
	pos    int
}

func newChunkedReader(chunks ...string) *chunkedReader {
	r := &chunkedReader{}
	for _, c := range chunks {
		r.chunks = append(r.chunks, []byte(c))
	}
	return r
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.pos])
	r.pos++
	return n, nil
}

// failingReader returns errFailingRead after yielding some initial bytes,
// simulating a genuine I/O failure (e.g. a broken pipe) distinct from EOF.
type failingReader struct {
	data []byte
	pos  int
}

var errFailingRead = errors.New("simulated read failure")

func (f *failingReader) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, errFailingRead
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

// newTestSource builds an *eventSource over r, the same way runSession
// does, so tests exercise the real Kitty-keyboard and bracketed-paste
// parsing rather than a stand-in.
func newTestSource(t *testing.T, r io.Reader) *eventSource {
	t.Helper()
	rd, err := input.NewReader(r, "", 0)
	if err != nil {
		t.Fatalf("input.NewReader: %v", err)
	}
	t.Cleanup(func() { rd.Close() })
	return newEventSource(rd)
}

func TestNextMessage_PlainEnterSubmitsImmediately(t *testing.T) {
	rd := newTestSource(t, newChunkedReader("hello", "\r"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want a submitted message")
	}
	if msg != "hello" {
		t.Errorf("msg = %q, want %q", msg, "hello")
	}
}

func TestNextMessage_PlainEnterViaKittyCSIUSubmitsImmediately(t *testing.T) {
	// A real Kitty CSI-u sequence for a bare Enter keypress (code 13, no
	// modifiers): CSI 13 u.
	rd := newTestSource(t, newChunkedReader("hi", "\x1b[13u"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want a submitted message")
	}
	if msg != "hi" {
		t.Errorf("msg = %q, want %q", msg, "hi")
	}
}

func TestNextMessage_ShiftEnterInsertsNewlineWithoutSubmitting(t *testing.T) {
	// Real Kitty CSI-u sequence for Shift+Enter: CSI 13 ; 2 u (modifier
	// value 2 = shift, encoded as mod-1 per the protocol).
	rd := newTestSource(t, newChunkedReader("line one", "\x1b[13;2u", "line two", "\r"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want a submitted message")
	}
	want := "line one\nline two"
	if msg != want {
		t.Errorf("msg = %q, want %q", msg, want)
	}
}

func TestNextMessage_CtrlJInsertsNewlineWithoutSubmitting(t *testing.T) {
	// Ctrl+J is a literal LF byte (0x0A) — the universal fallback for
	// terminals without Kitty keyboard support.
	rd := newTestSource(t, newChunkedReader("line one", "\n", "line two", "\r"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want a submitted message")
	}
	want := "line one\nline two"
	if msg != want {
		t.Errorf("msg = %q, want %q", msg, want)
	}
}

func TestNextMessage_BracketedPasteSubmitsAsSingleMessage(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "with trailing newline", content: "first line\nsecond line\n"},
		{name: "without trailing newline", content: "first line\nsecond line"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paste := "\x1b[200~" + tc.content + "\x1b[201~"
			rd := newTestSource(t, newChunkedReader(paste))

			msg, quit, err := nextMessage(rd, io.Discard)
			if err != nil {
				t.Fatalf("nextMessage returned error: %v", err)
			}
			if quit {
				t.Fatal("quit = true, want a submitted message")
			}
			if msg != tc.content {
				t.Errorf("msg = %q, want %q", msg, tc.content)
			}
		})
	}
}

func TestNextMessage_PasteFoldsInAlreadyTypedContent(t *testing.T) {
	// A user typing part of a message, then pasting, must not lose the
	// typed prefix — the paste submits everything accumulated so far,
	// typed and pasted alike.
	rd := newTestSource(t, newChunkedReader("before ", "\x1b[200~pasted\x1b[201~"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want a submitted message")
	}
	if want := "before pasted"; msg != want {
		t.Errorf("msg = %q, want %q", msg, want)
	}
}

func TestNextMessage_EchoesPastedContent(t *testing.T) {
	// Raw mode disables the terminal's own local echo, so without this a
	// pasted block would be invisible on screen even though it's captured
	// and submitted correctly.
	rd := newTestSource(t, newChunkedReader("\x1b[200~pasted\x1b[201~"))

	var echo bytes.Buffer
	if _, _, err := nextMessage(rd, &echo); err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if want := "pasted" + crlf; echo.String() != want {
		t.Errorf("echo = %q, want %q", echo.String(), want)
	}
}

func TestNextMessage_BracketedPasteThenManualTypingDoesNotResubmit(t *testing.T) {
	// This is exactly the ambiguity the old bufio.Reader.Buffered()
	// heuristic couldn't resolve (see ADR 0002): a paste with no trailing
	// newline, immediately followed by more manual typing. Bracketed paste
	// gives an unambiguous end marker, so the paste submits on its own and
	// the following keystrokes start a fresh message.
	rd := newTestSource(t, newChunkedReader("\x1b[200~pasted\x1b[201~", "typed", "\r"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit || msg != "pasted" {
		t.Fatalf("first nextMessage = (%q, quit=%v), want (\"pasted\", false)", msg, quit)
	}

	msg, quit, err = nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit || msg != "typed" {
		t.Fatalf("second nextMessage = (%q, quit=%v), want (\"typed\", false)", msg, quit)
	}
}

func TestNextMessage_BackspaceErasesLastTypedCharacter(t *testing.T) {
	rd := newTestSource(t, newChunkedReader("helllo", "\x7f\x7f", "o\r"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want a submitted message")
	}
	if msg != "hello" {
		t.Errorf("msg = %q, want %q", msg, "hello")
	}
}

func TestNextMessage_EnterOnEmptyBufferDoesNotSubmit(t *testing.T) {
	rd := newTestSource(t, newChunkedReader("\r", "\r", "hi\r"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want a submitted message")
	}
	if msg != "hi" {
		t.Errorf("msg = %q, want %q: bare Enter presses should be ignored, not submitted", msg, "hi")
	}
}

func TestNextMessage_ExitAndQuitEndSession(t *testing.T) {
	for _, line := range []string{"/exit", "/quit"} {
		t.Run(line, func(t *testing.T) {
			rd := newTestSource(t, newChunkedReader(line, "\r"))

			msg, quit, err := nextMessage(rd, io.Discard)
			if err != nil {
				t.Fatalf("nextMessage returned error: %v", err)
			}
			if !quit {
				t.Fatalf("quit = false, want true for a literal %q message", line)
			}
			if msg != "" {
				t.Errorf("msg = %q, want empty on session end", msg)
			}
		})
	}
}

func TestNextMessage_CtrlCEndsSessionAbandoningPartialInput(t *testing.T) {
	rd := newTestSource(t, newChunkedReader("some partial input", "\x03"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if !quit {
		t.Fatal("quit = false, want true on Ctrl+C")
	}
	if msg != "" {
		t.Errorf("msg = %q, want empty: Ctrl+C should abandon partial input", msg)
	}
}

func TestNextMessage_CtrlDOnEmptyBufferEndsSession(t *testing.T) {
	rd := newTestSource(t, newChunkedReader("\x04"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if !quit {
		t.Fatal("quit = false, want true on Ctrl+D with an empty buffer")
	}
	if msg != "" {
		t.Errorf("msg = %q, want empty", msg)
	}
}

func TestNextMessage_CtrlDWithPartialInputSubmitsInstead(t *testing.T) {
	// Matches canonical-mode/bash/Python-REPL convention (ADR 0007): Ctrl+D
	// only ends the session on an empty buffer. With something typed, it
	// submits exactly like Enter rather than discarding the input.
	rd := newTestSource(t, newChunkedReader("hello", "\x04"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want Ctrl+D on a non-empty buffer to submit, not quit")
	}
	if msg != "hello" {
		t.Errorf("msg = %q, want %q", msg, "hello")
	}
}

func TestNextMessage_CtrlDWithAccumulatedMultilineBufferSubmitsInstead(t *testing.T) {
	// The buffer counts as non-empty even when the current line is empty
	// but earlier lines were accumulated via Shift+Enter/Ctrl+J.
	rd := newTestSource(t, newChunkedReader("line one", "\x1b[13;2u", "\x04"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("quit = true, want Ctrl+D to submit accumulated multi-line input, not quit")
	}
	if msg != "line one\n" {
		t.Errorf("msg = %q, want %q", msg, "line one\n")
	}
}

func TestNextMessage_EOFWithNoInputEndsSession(t *testing.T) {
	rd := newTestSource(t, newChunkedReader())

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if !quit {
		t.Fatal("quit = false, want true on immediate EOF")
	}
	if msg != "" {
		t.Errorf("msg = %q, want empty", msg)
	}
}

func TestNextMessage_EOFAfterPartialInputEndsSessionWithoutSubmitting(t *testing.T) {
	rd := newTestSource(t, newChunkedReader("unterminated input"))

	msg, quit, err := nextMessage(rd, io.Discard)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if !quit {
		t.Fatal("quit = false, want true on EOF, even with unterminated accumulated input")
	}
	if msg != "" {
		t.Errorf("msg = %q, want empty: unterminated input must not be submitted on EOF", msg)
	}
}

func TestNextMessage_GenuineReadErrorIsPropagatedNotSwallowed(t *testing.T) {
	rd := newTestSource(t, &failingReader{data: []byte("partial")})

	_, _, err := nextMessage(rd, io.Discard)
	if !errors.Is(err, errFailingRead) {
		t.Fatalf("err = %v, want it to wrap %v: a genuine read error must not be treated the same as a clean EOF", err, errFailingRead)
	}
}

func TestResolveSkillCommand_MatchesDiscoveredSkillWithNoTrailingText(t *testing.T) {
	skills := []skill.Skill{
		{Name: "pdf-processing", Path: "/skills/pdf-processing"},
	}

	got, text, ok := resolveSkillCommand(skills, "/pdf-processing")
	if !ok {
		t.Fatal("ok = false, want true for a name matching a discovered skill")
	}
	if got.Name != "pdf-processing" {
		t.Errorf("matched = %+v, want the pdf-processing skill", got)
	}
	if text != "" {
		t.Errorf("text = %q, want empty when nothing follows the name", text)
	}
}

func TestResolveSkillCommand_PassesTrailingTextThroughUnchanged(t *testing.T) {
	skills := []skill.Skill{{Name: "pdf-processing", Path: "/skills/pdf-processing"}}

	got, text, ok := resolveSkillCommand(skills, "/pdf-processing summarize report.pdf $1")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got.Name != "pdf-processing" {
		t.Errorf("matched = %+v, want the pdf-processing skill", got)
	}
	if want := "summarize report.pdf $1"; text != want {
		t.Errorf("text = %q, want %q verbatim: no positional-argument substitution", text, want)
	}
}

func TestResolveSkillCommand_MultiLineTrailingTextStillResolves(t *testing.T) {
	// nextMessage lets a submitted message span multiple lines (Shift+Enter,
	// Ctrl+J), so the name must end at the first newline too, not just a
	// space, or a multi-line invocation would silently fail to resolve.
	skills := []skill.Skill{{Name: "pdf-processing", Path: "/skills/pdf-processing"}}

	got, text, ok := resolveSkillCommand(skills, "/pdf-processing\nsummarize this\nacross two lines")
	if !ok {
		t.Fatal("ok = false, want true: a newline after the name must still resolve it")
	}
	if got.Name != "pdf-processing" {
		t.Errorf("matched = %+v, want the pdf-processing skill", got)
	}
	if want := "summarize this\nacross two lines"; text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
}

func TestResolveSkillCommand_DisableModelInvocationSkillStillResolves(t *testing.T) {
	skills := []skill.Skill{
		{Name: "internal-only", Path: "/skills/internal-only", DisableModelInvocation: true},
	}

	got, _, ok := resolveSkillCommand(skills, "/internal-only")
	if !ok {
		t.Fatal("ok = false, want true: disable-model-invocation must not block explicit /name invocation")
	}
	if got.Name != "internal-only" {
		t.Errorf("matched = %+v, want the internal-only skill", got)
	}
}

func TestResolveSkillCommand_UnknownNameIsNotOK(t *testing.T) {
	skills := []skill.Skill{{Name: "pdf-processing", Path: "/skills/pdf-processing"}}

	_, _, ok := resolveSkillCommand(skills, "/no-such-skill")
	if ok {
		t.Fatal("ok = true, want false: no discovered skill named no-such-skill")
	}
}

func TestResolveSkillCommand_NonSlashMessageIsNotOK(t *testing.T) {
	skills := []skill.Skill{{Name: "pdf-processing", Path: "/skills/pdf-processing"}}

	_, _, ok := resolveSkillCommand(skills, "pdf-processing")
	if ok {
		t.Fatal("ok = true, want false: a message with no leading / is never a skill invocation")
	}
}

func TestResolveSkillCommand_EmptySkillSetIsNotOK(t *testing.T) {
	_, _, ok := resolveSkillCommand(nil, "/pdf-processing")
	if ok {
		t.Fatal("ok = true, want false when no skills were discovered at all")
	}
}

func TestHandoff_DrainsReplayBeforeChannel(t *testing.T) {
	ch := make(chan eventOrErr, 1)
	ch <- eventOrErr{ev: input.KeyPressEvent{Text: "b"}}

	h := &handoff{
		ch:     ch,
		replay: []eventOrErr{{ev: input.KeyPressEvent{Text: "a"}}},
	}

	ev, err := h.next()
	if err != nil {
		t.Fatalf("first next: %v", err)
	}
	if key, ok := ev.(input.KeyPressEvent); !ok || key.Text != "a" {
		t.Fatalf("first next = %#v, want the replayed event ahead of the channel", ev)
	}

	ev, err = h.next()
	if err != nil {
		t.Fatalf("second next: %v", err)
	}
	if key, ok := ev.(input.KeyPressEvent); !ok || key.Text != "b" {
		t.Fatalf("second next = %#v, want the channel event once replay is drained", ev)
	}
}

// waitForLeftover waits for watchForCtrlC's result, failing the test if it
// doesn't return promptly.
func waitForLeftover(t *testing.T, resultCh <-chan []eventOrErr) []eventOrErr {
	t.Helper()
	select {
	case leftover := <-resultCh:
		return leftover
	case <-time.After(2 * time.Second):
		t.Fatal("watchForCtrlC did not return promptly")
		return nil
	}
}

func TestWatchForCtrlC_CancelsAndReturnsLeftoverOnCtrlC(t *testing.T) {
	ch := make(chan eventOrErr)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan []eventOrErr, 1)
	go func() {
		resultCh <- watchForCtrlC(ch, cancel, done)
	}()

	ch <- eventOrErr{ev: input.KeyPressEvent{Text: "x"}}
	ch <- eventOrErr{ev: input.KeyPressEvent{Text: "y"}}
	ch <- eventOrErr{ev: input.KeyPressEvent{Mod: input.ModCtrl, Code: 'c'}}

	leftover := waitForLeftover(t, resultCh)

	if ctx.Err() == nil {
		t.Error("ctx.Err() = nil, want the turn's context cancelled after seeing Ctrl+C")
	}
	if len(leftover) != 2 {
		t.Fatalf("leftover = %#v, want exactly the 2 non-Ctrl+C events read before Ctrl+C", leftover)
	}
	if key, ok := leftover[0].ev.(input.KeyPressEvent); !ok || key.Text != "x" {
		t.Errorf("leftover[0] = %#v, want %q", leftover[0], "x")
	}
	if key, ok := leftover[1].ev.(input.KeyPressEvent); !ok || key.Text != "y" {
		t.Errorf("leftover[1] = %#v, want %q", leftover[1], "y")
	}
}

func TestWatchForCtrlC_StopsOnDoneWithoutCancelling(t *testing.T) {
	ch := make(chan eventOrErr)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan []eventOrErr, 1)
	go func() {
		resultCh <- watchForCtrlC(ch, cancel, done)
	}()

	ch <- eventOrErr{ev: input.KeyPressEvent{Text: "z"}}
	close(done)

	leftover := waitForLeftover(t, resultCh)

	if ctx.Err() != nil {
		t.Error("ctx.Err() != nil, want the turn's context untouched when the turn ends without Ctrl+C")
	}
	if len(leftover) != 1 {
		t.Fatalf("leftover = %#v, want the 1 event read before done closed", leftover)
	}
	if key, ok := leftover[0].ev.(input.KeyPressEvent); !ok || key.Text != "z" {
		t.Errorf("leftover[0] = %#v, want %q", leftover[0], "z")
	}
}

func TestWatchForCtrlC_TerminalErrorEndsWatchAndIsReturnedAsLeftover(t *testing.T) {
	ch := make(chan eventOrErr)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan []eventOrErr, 1)
	go func() {
		resultCh <- watchForCtrlC(ch, cancel, done)
	}()

	ch <- eventOrErr{err: io.EOF}

	leftover := waitForLeftover(t, resultCh)

	if ctx.Err() != nil {
		t.Error("ctx.Err() != nil, want a terminal read error left uncancelled: it's not a Ctrl+C")
	}
	if len(leftover) != 1 || !errors.Is(leftover[0].err, io.EOF) {
		t.Fatalf("leftover = %#v, want it to carry the terminal error so the handoff can surface it later", leftover)
	}
}

func TestNextMessage_EchoesTypedTextNewlinesAndBackspace(t *testing.T) {
	rd := newTestSource(t, newChunkedReader("hix", "\x7f", "\x1b[13;2u", "y", "\r"))

	var echo bytes.Buffer
	msg, _, err := nextMessage(rd, &echo)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if msg != "hi\ny" {
		t.Fatalf("msg = %q, want %q", msg, "hi\ny")
	}

	want := "hix" + "\b \b" + "\r\n" + "y" + "\r\n"
	if echo.String() != want {
		t.Errorf("echo = %q, want %q", echo.String(), want)
	}
}
