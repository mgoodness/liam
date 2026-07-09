package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestNextMessage_BlankLineTerminatesMultiLineMessage(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("line one\nline two\n\nnext message\n"))

	msg, quit, err := nextMessage(r)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("nextMessage reported quit, want a submitted message")
	}
	want := "line one\nline two"
	if msg != want {
		t.Errorf("msg = %q, want %q", msg, want)
	}
}

func TestNextMessage_ExitAndQuitEndSession(t *testing.T) {
	for _, line := range []string{"/exit", "/quit"} {
		t.Run(line, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(line + "\n"))

			msg, quit, err := nextMessage(r)
			if err != nil {
				t.Fatalf("nextMessage returned error: %v", err)
			}
			if !quit {
				t.Fatalf("quit = false, want true for a literal %q line", line)
			}
			if msg != "" {
				t.Errorf("msg = %q, want empty on session end", msg)
			}
		})
	}
}

func TestNextMessage_ExitMidMessageAbandonsAccumulatedLines(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("some partial input\n/exit\n"))

	msg, quit, err := nextMessage(r)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if !quit {
		t.Fatal("quit = false, want true when /exit appears mid-message")
	}
	if msg != "" {
		t.Errorf("msg = %q, want empty: /exit should abandon prior accumulated lines", msg)
	}
}

func TestNextMessage_EOFWithNoInputEndsSession(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))

	msg, quit, err := nextMessage(r)
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
	// No trailing blank line before EOF: the message was never terminated,
	// so per the Ctrl+D-ends-session contract it must not be submitted.
	r := bufio.NewReader(strings.NewReader("unterminated line"))

	msg, quit, err := nextMessage(r)
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

func TestNextMessage_LeadingBlankLinesAreIgnored(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\n\nreal message\n\n"))

	msg, quit, err := nextMessage(r)
	if err != nil {
		t.Fatalf("nextMessage returned error: %v", err)
	}
	if quit {
		t.Fatal("nextMessage reported quit, want a submitted message")
	}
	if msg != "real message" {
		t.Errorf("msg = %q, want %q", msg, "real message")
	}
}
