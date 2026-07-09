package main

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// nextMessage reads lines from r, accumulating them into a single message
// until a blank line is reached. It returns quit=true, with no message,
// when the session should end: a literal /exit or /quit line, or EOF. A
// genuine read error (anything but io.EOF) is returned as-is rather than
// treated as a clean session end.
func nextMessage(r *bufio.Reader) (msg string, quit bool, err error) {
	var lines []string
	for {
		line, readErr := r.ReadString('\n')
		line = strings.TrimRight(line, "\n")
		line = strings.TrimRight(line, "\r")

		switch strings.TrimSpace(line) {
		case "/exit", "/quit":
			return "", true, nil
		case "":
			if len(lines) > 0 {
				return strings.Join(lines, "\n"), false, nil
			}
		default:
			lines = append(lines, line)
		}

		if errors.Is(readErr, io.EOF) {
			return "", true, nil
		}
		if readErr != nil {
			return "", false, readErr
		}
	}
}
