package tool

import (
	"fmt"
	"strings"

	"github.com/mgoodness/liam/internal/render"
)

// formatHeader builds the "Found N <noun>[ (more <plural> available)]"
// header line shared by formatGrepResults/formatFindResults; shown is how
// many of total are actually included in the result.
func formatHeader(singular, plural string, total, shown int) string {
	header := fmt.Sprintf("Found %d %s", total, render.Pluralize(total, singular, plural))
	if shown < total {
		header += fmt.Sprintf(" (more %s available)", plural)
	}
	return header
}

// formatTruncationNotice builds the "[... truncated, N more <noun> not
// shown ...]" marker appended when hidden (total-shown) is positive,
// matching truncate()'s own bracket-marker convention (ADR-0005).
func formatTruncationNotice(singular, plural string, hidden int) string {
	return fmt.Sprintf("\n\n[... truncated, %d more %s not shown ...]", hidden, render.Pluralize(hidden, singular, plural))
}

// formatGrepResults renders matches into liam's shared find/grep output
// convention (ticket #18's resolution, matching OpenCode's own proven
// shape): a header count line, results grouped by file, "Line <n>: <text>"
// rows, and a truncation notice appended when total exceeds len(matches).
func formatGrepResults(matches []GrepMatch, total int) string {
	header := formatHeader("match", "matches", total, len(matches))
	if total == 0 {
		return header + "."
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")

	currentFile := ""
	for _, m := range matches {
		if m.File != currentFile {
			if currentFile != "" {
				b.WriteByte('\n')
			}
			b.WriteString(m.File)
			b.WriteByte('\n')
			currentFile = m.File
		}
		fmt.Fprintf(&b, "Line %d: %s\n", m.Line, m.Text)
	}

	out := strings.TrimRight(b.String(), "\n")
	if hidden := total - len(matches); hidden > 0 {
		out += formatTruncationNotice("match", "matches", hidden)
	}
	return out
}

// formatFindResults is formatGrepResults' counterpart for Find: paths
// carry no line numbers, so each result is just its own row grouped under
// the same header-and-truncation-notice convention.
func formatFindResults(paths []string, total int) string {
	header := formatHeader("file", "files", total, len(paths))
	if total == 0 {
		return header + "."
	}

	out := header + "\n\n" + strings.Join(paths, "\n")
	if hidden := total - len(paths); hidden > 0 {
		out += formatTruncationNotice("file", "files", hidden)
	}
	return out
}
