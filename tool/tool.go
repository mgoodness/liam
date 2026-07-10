// Package tool implements liam's Tools — read, write, edit, bash, and
// web_fetch, always available, plus the conditionally-available
// web_search — behind a dispatch table keyed by tool name.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

// Tools is the always-available core dispatch table: read, write, edit,
// bash, and web_fetch, present in every tool set New builds regardless of
// environment.
var Tools = map[string]Tool{
	"read":      {Definition: readDefinition, Handler: Read},
	"write":     {Definition: writeDefinition, Handler: Write},
	"edit":      {Definition: editDefinition, Handler: Edit},
	"bash":      {Definition: bashDefinition, Handler: Bash},
	"web_fetch": {Definition: webFetchDefinition, Handler: WebFetch},
}

// New assembles the dispatch table for a session: the four core Tools,
// plus web_search when braveAPIKey is non-empty. web_search is omitted
// entirely when no key is configured, rather than included as a tool
// definition that would then error on every call. Callers pass the
// returned map to Call explicitly rather than relying on package-level
// state, so the tool set assembled at startup is what actually gets
// dispatched — not a static map some caller has to remember to mutate.
func New(braveAPIKey string) map[string]Tool {
	tools := make(map[string]Tool, len(Tools)+1)
	for name, t := range Tools {
		tools[name] = t
	}
	if braveAPIKey != "" {
		tools["web_search"] = NewWebSearchTool(braveAPIKey, "")
	}
	return tools
}

// Definitions returns tools' Definitions in stable, name-sorted order, so
// a Provider's request body doesn't vary across runs due to Go's
// randomized map iteration.
func Definitions(tools map[string]Tool) []Definition {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, len(names))
	for i, name := range names {
		defs[i] = tools[name].Definition
	}
	return defs
}

// ErrUnknownTool is wrapped into the error Call returns when name isn't in
// tools, so callers can tell this apart from a Handler's own failure via
// errors.Is: an unknown tool name signals a bug in the caller (requesting
// a tool outside the definitions it was given), not a tool-level failure
// meant to be relayed back to the model.
var ErrUnknownTool = errors.New("unknown tool")

// Call looks up name in tools and invokes its Handler with args, truncating
// the result to MaxResultBytes. tools should be the same set (typically
// from New) whose Definitions were offered to the Provider, so a name the
// model was never given can't dispatch to something else. An unknown tool
// name is a Go error rather than a tool result, since the Provider should
// only ever request tools from the definitions it was given.
func Call(ctx context.Context, tools map[string]Tool, name string, args json.RawMessage) (string, error) {
	t, ok := tools[name]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownTool, name)
	}
	result, err := t.Handler(ctx, args)
	if err != nil {
		return "", err
	}
	return truncate(result), nil
}

// truncate caps s to MaxResultBytes, appending a marker noting how much
// was omitted.
func truncate(s string) string {
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
