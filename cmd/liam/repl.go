// Command liam is the REPL and CLI wiring that ties the agent loop to a
// real terminal session and the real Copilot Provider.
package main

import (
	"bufio"
	"strings"
)

// nextMessage reads lines from r, accumulating them into a single message
// until a blank line is reached. It returns quit=true, with no message,
// when the session should end: a literal /exit or /quit line, or EOF.
func nextMessage(r *bufio.Reader) (msg string, quit bool, err error) {
	var lines []string
	for {
		line, err := r.ReadString('\n')
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

		if err != nil {
			return "", true, nil
		}
	}
}
