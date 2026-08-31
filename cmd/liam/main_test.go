package main

import (
	"bytes"
	"context"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
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
	t.Setenv("HOME", t.TempDir()) // isolate from the real machine's ~/.agents/skills

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

// TestRunInteractiveQuitFiresSessionEndHookExactlyOnce covers issue #45's
// sessionEnd lifecycle point end-to-end in interactive mode: runInteractive
// guarantees it fires once when the program exits, regardless of which
// quit path (here, "/quit") got it there — a project liam.jsonc configures
// a sessionEnd hook that appends a line to a marker file, and "/quit" must
// cause exactly one line to land.
func TestRunInteractiveQuitFiresSessionEndHookExactlyOnce(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	cwd := t.TempDir()
	markerPath := filepath.Join(cwd, "ended")
	configContent := []byte(`{
  "hooks": { "sessionEnd": [{ "command": "echo x >> ` + markerPath + `" }] }
}`)
	if err := os.WriteFile(filepath.Join(cwd, "liam.jsonc"), configContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Chdir(cwd)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("/quit\r")

	done := make(chan int, 1)
	go func() { done <- run(nil, stdin, &stdout, &stderr) }()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within 5s of \"/quit\" being submitted")
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("sessionEnd hook did not run: %v", err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 1 {
		t.Errorf("sessionEnd hook ran %d times, want exactly 1", lines)
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

	req := buildRequest(cfg, "hello", "")
	if req.Model != "openai/gpt-4o" {
		t.Errorf("req.Model = %q, want %q", req.Model, "openai/gpt-4o")
	}
}

// TestBuildRequestThreadsSystemPrompt covers -skill's force-activation
// path: a force-activated skill's body reaches the request as
// SystemPrompt.
func TestBuildRequestThreadsSystemPrompt(t *testing.T) {
	req := buildRequest(config.Config{}, "hello", "skill instructions")
	if req.SystemPrompt != "skill instructions" {
		t.Errorf("req.SystemPrompt = %q, want %q", req.SystemPrompt, "skill instructions")
	}
}

// doneProvider is a minimal provider.Provider that immediately yields a
// DoneEvent, standing in for a real model backend in headless-mode tests
// that only care about hook lifecycle wiring, not turn content.
type doneProvider struct{}

func (doneProvider) Name() string { return "done" }
func (doneProvider) Stream(context.Context, provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		yield(provider.DoneEvent{FinishReason: "stop"}, nil)
	}
}

// TestRunHeadlessFiresSessionStartAndSessionEndHooks covers issue #45's
// sessionStart/sessionEnd lifecycle points in headless mode: both must fire
// exactly once, bracketing the single turn.
func TestRunHeadlessFiresSessionStartAndSessionEndHooks(t *testing.T) {
	dir := t.TempDir()
	startedPath := filepath.Join(dir, "started")
	endedPath := filepath.Join(dir, "ended")
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		SessionStart: []config.HookConfig{{Command: "touch " + startedPath}},
		SessionEnd:   []config.HookConfig{{Command: "touch " + endedPath}},
	}}
	loop := agent.Loop{Provider: doneProvider{}, Hooks: hooks}

	var stdout, stderr bytes.Buffer
	code := runHeadless(loop, config.Config{}, "hi", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runHeadless() = %d, want 0; stderr = %q", code, stderr.String())
	}

	if _, err := os.Stat(startedPath); err != nil {
		t.Errorf("sessionStart hook did not run: %v", err)
	}
	if _, err := os.Stat(endedPath); err != nil {
		t.Errorf("sessionEnd hook did not run: %v", err)
	}
	if hooks.SessionID == "" {
		t.Error("hooks.SessionID was never set")
	}
}

// isolateSkillDirs points HOME/XDG_CONFIG_HOME/XDG_STATE_HOME at fresh
// temp dirs so skill discovery/trust tests never touch the real
// developer machine's ~/.agents/skills or trust store, and returns the
// fresh HOME.
func isolateSkillDirs(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return home
}

func writeSkillFixture(t *testing.T, dir, name string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: a test skill\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestDiscoverSkillsHeadlessDefaultsUntrusted covers issue #53's headless
// non-interactive equivalent for the project trust prompt: with no
// skills.trustProjectSkills override and interactive=false (as in -p
// mode), project-scope skills are not loaded.
func TestDiscoverSkillsHeadlessDefaultsUntrusted(t *testing.T) {
	isolateSkillDirs(t)
	cwd := t.TempDir()
	writeSkillFixture(t, filepath.Join(cwd, ".agents", "skills"), "proj-skill")

	var stdout, stderr bytes.Buffer
	skills, err := discoverSkills(cwd, config.Config{}, false, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("skills = %+v, want none (untrusted project, headless mode)", skills)
	}
}

// TestDiscoverSkillsHeadlessRespectsTrustOverride covers the documented
// config-flag escape hatch for headless mode.
func TestDiscoverSkillsHeadlessRespectsTrustOverride(t *testing.T) {
	isolateSkillDirs(t)
	cwd := t.TempDir()
	writeSkillFixture(t, filepath.Join(cwd, ".agents", "skills"), "proj-skill")

	trust := true
	cfg := config.Config{Skills: config.SkillsConfig{TrustProjectSkills: &trust}}

	var stdout, stderr bytes.Buffer
	skills, err := discoverSkills(cwd, cfg, false, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "proj-skill" {
		t.Fatalf("skills = %+v, want [proj-skill] (trustProjectSkills: true)", skills)
	}
}

// TestDiscoverSkillsInteractivePromptsAndPersistsDecision covers the
// interactive one-time trust prompt: answering "y" loads project skills
// and persists the decision so a second call (simulating a later run) in
// the same project doesn't prompt again.
func TestDiscoverSkillsInteractivePromptsAndPersistsDecision(t *testing.T) {
	isolateSkillDirs(t)
	cwd := t.TempDir()
	writeSkillFixture(t, filepath.Join(cwd, ".agents", "skills"), "proj-skill")

	var stdout, stderr bytes.Buffer
	skills, err := discoverSkills(cwd, config.Config{}, true, strings.NewReader("y\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "proj-skill" {
		t.Fatalf("skills = %+v, want [proj-skill] after answering \"y\"", skills)
	}
	if !strings.Contains(stdout.String(), "trust project-level skills") {
		t.Errorf("stdout = %q, want a trust prompt", stdout.String())
	}

	// Second call, no stdin available to answer a prompt: the persisted
	// "trusted" decision must be reused rather than defaulting to false.
	stdout.Reset()
	skills, err = discoverSkills(cwd, config.Config{}, true, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("discoverSkills (second call): %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "proj-skill" {
		t.Fatalf("skills = %+v, want [proj-skill] reused from the persisted decision", skills)
	}
	if strings.Contains(stdout.String(), "trust project-level skills") {
		t.Error("second call re-prompted; want the persisted decision reused")
	}
}

// TestDiscoverSkillsLogsDiagnosticsToStderr covers the security-scan
// diagnostic surfacing distinctly from a trust denial (issue #53's
// content-level check).
func TestDiscoverSkillsLogsDiagnosticsToStderr(t *testing.T) {
	home := isolateSkillDirs(t)
	skillDir := filepath.Join(home, ".agents", "skills", "bad")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: bad\n---\nno description\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	skills, err := discoverSkills(t.TempDir(), config.Config{}, false, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("skills = %+v, want none", skills)
	}
	if !strings.Contains(stderr.String(), "description") {
		t.Errorf("stderr = %q, want a diagnostic about the missing description", stderr.String())
	}
}

// TestRunSkillFlagRequiresHeadlessMode covers -skill's documented
// restriction: it force-activates a skill ahead of a headless prompt, so
// it requires -p.
func TestRunSkillFlagRequiresHeadlessMode(t *testing.T) {
	isolateSkillDirs(t)
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-skill", "foo"}, strings.NewReader(""), &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-skill requires -p") {
		t.Errorf("stderr = %q, want a mention that -skill requires -p", stderr.String())
	}
}

// TestRunSkillFlagUnknownSkillErrors covers -skill naming a skill that
// wasn't discovered.
func TestRunSkillFlagUnknownSkillErrors(t *testing.T) {
	isolateSkillDirs(t)
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-p", "hi", "-skill", "nonexistent"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown skill "nonexistent"`) {
		t.Errorf("stderr = %q, want a mention of the unknown skill", stderr.String())
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
