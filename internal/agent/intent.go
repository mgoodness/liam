package agent

import (
	"encoding/json"

	"github.com/mgoodness/liam/internal/tool"
)

// intentProperty is the schema property withIntent injects into every
// Tool's advertised Parameters, and the argument key extractIntent/dispatch
// read it back from — issue #63's required, model-supplied justification
// for a call, recorded verbatim on that call's Trace line (see
// docs/research/pi-go-jcode-prior-art.md finding #7, jcode's own required
// "intent" schema property).
const intentProperty = "intent"

// withIntent returns a copy of schema with intentProperty injected as a
// required string property — every Tool's advertised parameters gain it
// uniformly here, rather than each Tool implementation (built-in or MCP)
// having to declare it itself. schema itself is never mutated: it may be a
// built-in Tool's package-level literal (shared across every call) or an
// MCP server's schema (shared across every session using that server).
func withIntent(schema tool.Schema) map[string]any {
	out := make(map[string]any, len(schema)+1)
	for k, v := range schema {
		out[k] = v
	}

	props, _ := out["properties"].(map[string]any)
	newProps := make(map[string]any, len(props)+1)
	for k, v := range props {
		newProps[k] = v
	}
	newProps[intentProperty] = map[string]any{
		"type":        "string",
		"description": "Why you're making this call right now — a short, specific justification recorded in the harness's Trace audit log.",
	}
	out["properties"] = newProps
	out["required"] = appendRequired(out["required"], intentProperty)
	return out
}

// appendRequired returns existing — a JSON Schema "required" array,
// decoded as either []string (every built-in Tool's own package-level
// literal) or []any (an MCP server's schema, generically JSON-decoded) —
// as a fresh []any with name appended, deduplicated. Any other/absent type
// for existing is treated as empty, matching a Tool that declared no
// required properties of its own.
func appendRequired(existing any, name string) []any {
	var out []any
	switch v := existing.(type) {
	case []string:
		for _, s := range v {
			out = append(out, s)
		}
	case []any:
		out = append(out, v...)
	}
	for _, v := range out {
		if s, ok := v.(string); ok && s == name {
			return out
		}
	}
	return append(out, name)
}

// extractIntent pulls the "intent" property (see withIntent) out of a tool
// call's raw argument JSON, best-effort: malformed JSON or a missing/
// non-string "intent" simply yields "" rather than failing the call — the
// full args parse a few lines later in dispatch is what actually rejects
// malformed JSON.
func extractIntent(argsJSON string) string {
	if argsJSON == "" {
		return ""
	}
	var v struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &v); err != nil {
		return ""
	}
	return v.Intent
}
