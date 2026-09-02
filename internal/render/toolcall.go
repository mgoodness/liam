// Package render formats a dispatched tool call into the plain-text
// "name(args) → result" line shared by headless mode's stdout output and
// the TUI's inline tool-call rendering — the convention established by the
// find/grep/web_search tools' own plain-text output and documented in the
// TUI shell prototype's HEADLESS.md.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// maxResultLine caps how much of a tool result's first line is shown, so a
// large result (e.g. a full file read) doesn't blow out a single line.
const maxResultLine = 80

// ToolCall formats one tool call and its result as a single line, without
// the leading "⚙ " marker (callers place that themselves, since the TUI
// styles it and headless mode doesn't).
func ToolCall(name, argsJSON, content string, isError bool) string {
	return fmt.Sprintf("%s → %s", formatCall(name, argsJSON), summarizeResult(content, isError))
}

// formatCall renders "name(key: value, key2: value2)" from a tool call's
// name and raw JSON arguments, with keys sorted for deterministic output.
func formatCall(name, argsJSON string) string {
	var args map[string]any
	_ = json.Unmarshal([]byte(argsJSON), &args)

	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatArgValue(args[k])))
	}
	return fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", "))
}

func formatArgValue(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// summarizeResult reduces a tool result's content to a single truncated
// line, prefixed with "error: " for a failed call.
func summarizeResult(content string, isError bool) string {
	line := content
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i] + "…"
	}
	line = TruncateWidth(line, maxResultLine)
	if isError {
		return "error: " + line
	}
	return line
}
