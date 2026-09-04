package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestNewToolCallLineSetsDurationOnlyWhenExecuted(t *testing.T) {
	cases := []struct {
		name     string
		decision Decision
		want     int64
	}{
		{"executed", DecisionExecuted, 250},
		{"denied_by_hook", DecisionDeniedByHook, 0},
		{"errored", DecisionErrored, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewToolCallLine(fixedNow, "sess-1", "bash", "shell", tc.decision, "intent", "source", "reason", 250*time.Millisecond)
			if l.DurationMs != tc.want {
				t.Errorf("DurationMs = %d, want %d", l.DurationMs, tc.want)
			}
		})
	}
}

func TestNewToolCallLineFieldsAndJSONNames(t *testing.T) {
	l := NewToolCallLine(fixedNow, "sess-1", "bash", "shell", DecisionExecuted, "checking disk usage", "", "", 42*time.Millisecond)

	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	wantKeys := map[string]any{
		"ts":          "2026-01-02T03:04:05Z",
		"session_id":  "sess-1",
		"tool":        "bash",
		"side_effect": "shell",
		"decision":    "executed",
		"intent":      "checking disk usage",
		"duration_ms": float64(42),
	}
	for k, want := range wantKeys {
		if got := m[k]; got != want {
			t.Errorf("field %q = %v, want %v", k, got, want)
		}
	}
	// source/reason are omitempty and both empty here.
	if _, ok := m["source"]; ok {
		t.Errorf("source = %v, want omitted when empty", m["source"])
	}
	if _, ok := m["reason"]; ok {
		t.Errorf("reason = %v, want omitted when empty", m["reason"])
	}
}

func TestNewToolCallLineDeniedByHookCarriesSourceAndReason(t *testing.T) {
	l := NewToolCallLine(fixedNow, "sess-1", "bash", "shell", DecisionDeniedByHook, "rm everything", "./policy.sh", "no destructive commands", 0)
	if l.Source != "./policy.sh" {
		t.Errorf("Source = %q, want %q", l.Source, "./policy.sh")
	}
	if l.Reason != "no destructive commands" {
		t.Errorf("Reason = %q, want %q", l.Reason, "no destructive commands")
	}
	if l.Intent != "rm everything" {
		t.Errorf("Intent = %q, want %q, even on a denied call", l.Intent, "rm everything")
	}
}

func TestNewHookRunLineFieldsAndJSONNames(t *testing.T) {
	l := NewHookRunLine(fixedNow, "sess-1", "beforeTool", "./check.sh", 1, 15*time.Millisecond, "denied")

	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	wantKeys := map[string]any{
		"ts":          "2026-01-02T03:04:05Z",
		"session_id":  "sess-1",
		"lifecycle":   "beforeTool",
		"command":     "./check.sh",
		"exit_code":   float64(1),
		"duration_ms": float64(15),
		"stderr":      "denied",
	}
	for k, want := range wantKeys {
		if got := m[k]; got != want {
			t.Errorf("field %q = %v, want %v", k, got, want)
		}
	}
}

func TestNewHookRunLineOmitsEmptyStderr(t *testing.T) {
	l := NewHookRunLine(fixedNow, "sess-1", "afterTool", "./log.sh", 0, time.Millisecond, "")
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "stderr") {
		t.Errorf("marshaled = %s, want no stderr key when empty", data)
	}
}

func TestNewHookRunLineTruncatesLongStderr(t *testing.T) {
	long := strings.Repeat("x", stderrCap+500)
	l := NewHookRunLine(fixedNow, "sess-1", "beforeTool", "./check.sh", 1, time.Millisecond, long)

	if len(l.Stderr) >= len(long) {
		t.Fatalf("Stderr len = %d, want it truncated below the original %d bytes", len(l.Stderr), len(long))
	}
	if !strings.Contains(l.Stderr, "truncated") {
		t.Errorf("Stderr = %q, want a truncation marker", l.Stderr)
	}
}

func TestNewHookRunLineLeavesShortStderrUntouched(t *testing.T) {
	l := NewHookRunLine(fixedNow, "sess-1", "beforeTool", "./check.sh", 1, time.Millisecond, "short message")
	if l.Stderr != "short message" {
		t.Errorf("Stderr = %q, want it unchanged", l.Stderr)
	}
}

// readLines reads path (a Writer-produced JSONL file) and decodes each
// non-empty line into T.
func readLines[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
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

func TestWriterWritesToolCallLineToSessionFile(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	w := New()
	w.SessionID = "sess-abc"
	w.WriteToolCall("bash", "shell", DecisionExecuted, "list files", "", "", 10*time.Millisecond)
	w.Close()

	path := filepath.Join(stateHome, "liam", "traces", "sess-abc.jsonl")
	lines := readLines[ToolCallLine](t, path)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1", len(lines))
	}
	if lines[0].Tool != "bash" || lines[0].Decision != DecisionExecuted || lines[0].SessionID != "sess-abc" {
		t.Errorf("line = %+v, want the written ToolCallLine", lines[0])
	}
}

func TestWriterWritesHookRunLineToSessionFile(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	w := New()
	w.SessionID = "sess-abc"
	w.WriteHookRun("beforeTool", "./check.sh", 0, 5*time.Millisecond, "")
	w.Close()

	path := filepath.Join(stateHome, "liam", "traces", "sess-abc.jsonl")
	lines := readLines[HookRunLine](t, path)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1", len(lines))
	}
	if lines[0].Command != "./check.sh" || lines[0].Lifecycle != "beforeTool" {
		t.Errorf("line = %+v, want the written HookRunLine", lines[0])
	}
}

func TestWriterSwitchesFileOnSessionIDChange(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	w := New()
	w.SessionID = "sess-first"
	w.WriteToolCall("bash", "shell", DecisionExecuted, "", "", "", time.Millisecond)
	w.SessionID = "sess-second"
	w.WriteToolCall("read", "read", DecisionExecuted, "", "", "", time.Millisecond)
	w.Close()

	traces := filepath.Join(stateHome, "liam", "traces")
	firstLines := readLines[ToolCallLine](t, filepath.Join(traces, "sess-first.jsonl"))
	secondLines := readLines[ToolCallLine](t, filepath.Join(traces, "sess-second.jsonl"))
	if len(firstLines) != 1 || firstLines[0].Tool != "bash" {
		t.Errorf("sess-first.jsonl lines = %+v, want 1 line for bash", firstLines)
	}
	if len(secondLines) != 1 || secondLines[0].Tool != "read" {
		t.Errorf("sess-second.jsonl lines = %+v, want 1 line for read", secondLines)
	}
}

func TestWriterMultipleWritesAppendInOrder(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	w := New()
	w.SessionID = "sess-abc"
	for i := 0; i < 20; i++ {
		w.WriteToolCall("bash", "shell", DecisionExecuted, "", "", "", time.Millisecond)
	}
	w.Close()

	lines := readLines[ToolCallLine](t, filepath.Join(stateHome, "liam", "traces", "sess-abc.jsonl"))
	if len(lines) != 20 {
		t.Fatalf("len(lines) = %d, want 20", len(lines))
	}
}
