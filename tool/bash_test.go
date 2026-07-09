package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func callBash(t *testing.T, command string, timeoutSeconds int) (string, error) {
	t.Helper()
	args := map[string]any{"command": command}
	if timeoutSeconds != 0 {
		args["timeout"] = timeoutSeconds
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	return Bash(context.Background(), raw)
}

func TestBash_CapturesOutputAndExitCode(t *testing.T) {
	got, err := callBash(t, "echo out; echo err >&2; exit 3", 0)
	if err != nil {
		t.Fatalf("Bash: %v", err)
	}
	if !strings.Contains(got, "exit code: 3") {
		t.Errorf("expected exit code 3 in output, got %q", got)
	}
	if !strings.Contains(got, "out") {
		t.Errorf("expected stdout captured, got %q", got)
	}
	if !strings.Contains(got, "err") {
		t.Errorf("expected stderr captured, got %q", got)
	}
}

func TestBash_DefaultTimeoutKillsCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")

	args, err := json.Marshal(map[string]any{"command": "sleep 2 && touch " + marker})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}

	_, err = runBash(context.Background(), args, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("command should have been killed before creating marker")
	}
}

func TestBash_PerCallTimeoutOverride(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")

	_, err := callBash(t, "sleep 2 && touch "+marker, 1)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("command should have been killed before creating marker")
	}
}

func TestBash_MissingCommand(t *testing.T) {
	if _, err := callBash(t, "", 0); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestBashSummarize_ShowsCommand(t *testing.T) {
	args, err := json.Marshal(map[string]string{"command": "ls -la /tmp"})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	if got, want := bashSummarize(args), "ls -la /tmp"; got != want {
		t.Errorf("bashSummarize(%s) = %q, want %q", args, got, want)
	}
}
