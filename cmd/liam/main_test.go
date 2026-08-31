package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresPromptFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-p") {
		t.Errorf("stderr = %q, want a mention of -p", stderr.String())
	}
}

func TestRunRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-p", "hello"}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") {
		t.Errorf("stderr = %q, want a mention of OPENROUTER_API_KEY", stderr.String())
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-bogus"}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-version"}, &stdout, &stderr)

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
