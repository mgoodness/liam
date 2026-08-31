package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgoodness/liam/internal/config"
)

func TestRunRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-p", "hello"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") {
		t.Errorf("stderr = %q, want a mention of OPENROUTER_API_KEY", stderr.String())
	}
}

// TestRunNoArgsOpensInteractiveTUI is issue #57's headline behavior change:
// running liam with no arguments (no -p) opens the interactive TUI instead
// of erroring. It drives a real tea.Program with piped input/output (per
// Bubble Tea's own testing pattern — WithInput/WithOutput), scripting
// "/quit" + Enter and asserting the program exits cleanly rather than
// hanging or requiring -p.
func TestRunNoArgsOpensInteractiveTUI(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("/quit\r")

	done := make(chan int, 1)
	go func() { done <- run(nil, stdin, &stdout, &stderr) }()

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code = %d, want 0; stderr = %q", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within 5s of \"/quit\" being submitted")
	}
}

// TestConfigFileModelReachesBuildRequest is the config system's own
// end-to-end check (issue #43): a provider.model set in a project
// liam.jsonc, loaded the same way run() loads it, changes the model
// actually placed on the provider.Request that would be sent — not just the
// parsed Config value.
func TestConfigFileModelReachesBuildRequest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cwd := t.TempDir()
	configPath := filepath.Join(cwd, "liam.jsonc")
	content := []byte(`{
  // pin a specific model instead of the ticket-1 hardcoded default
  "provider": { "model": "openai/gpt-4o" }
}`)
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.Load(cwd, "")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	req := buildRequest(cfg, "hello")
	if req.Model != "openai/gpt-4o" {
		t.Errorf("req.Model = %q, want %q", req.Model, "openai/gpt-4o")
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-bogus"}, strings.NewReader(""), &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-version"}, strings.NewReader(""), &stdout, &stderr)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got := strings.TrimSpace(stdout.String()); got == "" {
		t.Error("stdout is empty, want a version string")
	}
	if stderr.String() != "" {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestVersionStringUsesLdflagsVersionWhenSet(t *testing.T) {
	old := version
	defer func() { version = old }()

	version = "v1.2.3"
	if got := versionString(); got != "v1.2.3" {
		t.Errorf("versionString() = %q, want %q", got, "v1.2.3")
	}
}

func TestVersionStringFallsBackToDev(t *testing.T) {
	old := version
	defer func() { version = old }()

	version = "dev"
	if got := versionString(); got == "" {
		t.Error("versionString() is empty, want a non-empty fallback")
	}
}

// TestVersionFallsBackToBuildInfo exercises the real debug.ReadBuildInfo
// path, which TestVersionStringFallsBackToDev cannot: within `go test`,
// build info always reports "(devel)", masking a broken fallback. A real
// `go build` (no ldflags) embeds a module pseudo-version instead.
func TestVersionFallsBackToBuildInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build integration test in short mode")
	}

	bin := filepath.Join(t.TempDir(), "liam")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("running built binary: %v\n%s", err, out)
	}

	if got := strings.TrimSpace(string(out)); got == "" || got == "dev" {
		t.Errorf("built binary -version = %q, want a build-info-derived version", got)
	}
}
