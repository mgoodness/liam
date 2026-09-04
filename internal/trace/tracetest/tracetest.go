// Package tracetest provides shared test helpers for packages that drive a
// real *trace.Writer and need to read the JSONL it wrote back —
// internal/hook and internal/agent both do this to verify issue #63's
// HookRunLine/ToolCallLine wiring end to end, rather than mocking Writer
// out.
package tracetest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/trace"
)

// New isolates a *trace.Writer under a fresh temp XDG_STATE_HOME, pointed
// at sessionID. It returns both the Writer (for the caller to attach to
// whatever it's testing) and stateHome (to hand back to ReadLines). Callers
// must call w.Close() themselves once done writing — deterministically
// draining pending writes — before reading the file back with ReadLines.
func New(t *testing.T, sessionID string) (w *trace.Writer, stateHome string) {
	t.Helper()
	stateHome = t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	w = trace.New()
	w.SessionID = sessionID
	return w, stateHome
}

// ReadLines reads sessionID's trace file under stateHome (as returned by
// New) and decodes every non-empty line as T — trace.ToolCallLine or
// trace.HookRunLine, depending on what the caller wrote.
func ReadLines[T any](t *testing.T, stateHome, sessionID string) []T {
	t.Helper()
	path := filepath.Join(stateHome, "liam", "traces", sessionID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading trace file: %v", err)
	}
	var out []T
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("unmarshaling line %q: %v", line, err)
		}
		out = append(out, v)
	}
	return out
}
