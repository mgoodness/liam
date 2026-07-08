// Package tool implements liam's v1 Tools — read, write, edit, and bash —
// behind a fixed dispatch table keyed by tool name.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// MaxResultBytes caps how much text a single tool result may return.
// Content beyond this is truncated with a marker rather than returned
// whole, since v1 has no context compaction and one unbounded read or
// `cat` would otherwise flood the model's context window.
const MaxResultBytes = 50 * 1024

// Definition is a hand-written, JSON-schema-shaped description of a tool,
// marshaled via encoding/json and sent to the Provider so the model knows
// what it can call and how.
type Definition struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

// Parameters describes a tool's arguments as a JSON Schema object.
type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single parameter within Parameters.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Handler executes a tool call given its raw JSON arguments and returns the
// result text to feed back to the model. An error represents a tool-level
// failure (bad args, no match, non-zero exit, timeout, ...) meant to be
// relayed to the model as the tool's result — not a crash of the session.
type Handler func(ctx context.Context, args json.RawMessage) (string, error)

// Tool pairs a tool's schema Definition with the Handler that executes it.
type Tool struct {
	Definition Definition
	Handler    Handler
}

// Tools is the fixed dispatch table mapping tool name to its Definition and
// Handler for the v1 tool set.
var Tools = map[string]Tool{
	"read":  {Definition: readDefinition, Handler: Read},
	"write": {Definition: writeDefinition, Handler: Write},
	"edit":  {Definition: editDefinition, Handler: Edit},
	"bash":  {Definition: bashDefinition, Handler: Bash},
}

// ErrUnknownTool is wrapped into the error Call returns when name isn't in
// Tools, so callers can tell this apart from a Handler's own failure via
// errors.Is: an unknown tool name signals a bug in the caller (requesting
// a tool outside the definitions it was given), not a tool-level failure
// meant to be relayed back to the model.
var ErrUnknownTool = errors.New("unknown tool")

// Call looks up name in Tools and invokes its Handler with args, truncating
// the result to MaxResultBytes. An unknown tool name is a Go error rather
// than a tool result, since the Provider should only ever request tools
// from the definitions it was given.
func Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := Tools[name]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownTool, name)
	}
	result, err := t.Handler(ctx, args)
	if err != nil {
		return "", err
	}
	return Truncate(result), nil
}

// Truncate caps s to MaxResultBytes, appending a marker noting how much
// was omitted. Exported so callers invoking a Tool's Handler directly
// (bypassing Call) can still apply the same cap.
func Truncate(s string) string {
	if len(s) <= MaxResultBytes {
		return s
	}

	// Back up from the byte cap to a rune boundary so we never split a
	// multibyte UTF-8 character in two, which would hand the model an
	// invalid string.
	cut := MaxResultBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	omitted := len(s) - cut
	return fmt.Sprintf("%s\n[truncated: %d bytes omitted]", s[:cut], omitted)
}
