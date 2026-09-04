package agent

import (
	"testing"

	"github.com/mgoodness/liam/internal/tool"
)

// TestToolDefsInjectIntentAsRequiredProperty covers issue #63's schema-
// injection acceptance criterion: every advertised Tool gains a required
// "intent" string property, regardless of what the Tool itself declared.
func TestToolDefsInjectIntentAsRequiredProperty(t *testing.T) {
	reg := tool.NewRegistry(&fakeTool{name: "bash"})
	defs := toolDefs(reg)
	if len(defs) != 1 {
		t.Fatalf("len(defs) = %d, want 1", len(defs))
	}

	props, ok := defs[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Parameters[properties] = %v, want a map", defs[0].Parameters["properties"])
	}
	if _, ok := props["intent"]; !ok {
		t.Fatalf("Parameters[properties] = %+v, want an \"intent\" entry", props)
	}

	required, ok := defs[0].Parameters["required"].([]any)
	if !ok {
		t.Fatalf("Parameters[required] = %v, want a []any", defs[0].Parameters["required"])
	}
	var sawIntent bool
	for _, r := range required {
		if r == "intent" {
			sawIntent = true
		}
	}
	if !sawIntent {
		t.Errorf("required = %+v, want \"intent\" included", required)
	}
}

// TestToolDefsInjectIntentPreservesExistingRequiredProperties covers the
// merge behavior against a Tool that already declares its own required
// properties (every built-in Tool's own []string-typed "required"), plus a
// stand-in for an MCP tool's []any-typed one.
func TestToolDefsInjectIntentPreservesExistingRequiredProperties(t *testing.T) {
	cases := []struct {
		name     string
		schema   tool.Schema
		wantReqs []string
	}{
		{
			name: "string-typed required (built-in Tool convention)",
			schema: tool.Schema{
				"type":       "object",
				"properties": map[string]any{"command": map[string]any{"type": "string"}},
				"required":   []string{"command"},
			},
			wantReqs: []string{"command", "intent"},
		},
		{
			name: "any-typed required (MCP tool convention)",
			schema: tool.Schema{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []any{"query"},
			},
			wantReqs: []string{"query", "intent"},
		},
		{
			name:     "no properties/required at all",
			schema:   tool.Schema{"type": "object"},
			wantReqs: []string{"intent"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withIntent(tc.schema)
			required, _ := got["required"].([]any)
			if len(required) != len(tc.wantReqs) {
				t.Fatalf("required = %+v, want %+v", required, tc.wantReqs)
			}
			for i, want := range tc.wantReqs {
				if required[i] != want {
					t.Errorf("required[%d] = %v, want %q", i, required[i], want)
				}
			}
		})
	}
}

// TestWithIntentNeverMutatesOriginalSchema covers withIntent's "schema
// itself is never mutated" invariant — a shared package-level literal
// (every built-in Tool's Parameters()) must come back out unchanged on a
// second call.
func TestWithIntentNeverMutatesOriginalSchema(t *testing.T) {
	schema := tool.Schema{
		"type":       "object",
		"properties": map[string]any{"command": map[string]any{"type": "string"}},
		"required":   []string{"command"},
	}
	_ = withIntent(schema)

	if _, ok := schema["properties"].(map[string]any)["intent"]; ok {
		t.Error("withIntent mutated the original schema's properties map")
	}
	if len(schema["required"].([]string)) != 1 {
		t.Error("withIntent mutated the original schema's required slice")
	}
}

// TestExtractIntentBestEffort covers extractIntent's tolerance for
// malformed/absent input — it never fails a call by itself; dispatch's own
// full args parse is what rejects malformed JSON.
func TestExtractIntentBestEffort(t *testing.T) {
	cases := []struct {
		name     string
		argsJSON string
		want     string
	}{
		{"present", `{"command":"ls","intent":"list files"}`, "list files"},
		{"empty string", "", ""},
		{"malformed json", "not json", ""},
		{"missing key", `{"command":"ls"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractIntent(tc.argsJSON); got != tc.want {
				t.Errorf("extractIntent(%q) = %q, want %q", tc.argsJSON, got, tc.want)
			}
		})
	}
}
