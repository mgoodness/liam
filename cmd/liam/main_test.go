package main

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/mgoodness/liam/internal/instructions"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
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

// fakeMCPTool is a minimal tool.Tool, standing in for a real MCP-sourced
// tool in fakeLoader-driven tests.
type fakeMCPTool struct{ name string }

func (f fakeMCPTool) Name() string            { return f.name }
func (f fakeMCPTool) Description() string     { return "fake mcp tool" }
func (f fakeMCPTool) Parameters() tool.Schema { return tool.Schema{"type": "object"} }
func (f fakeMCPTool) Safety() tool.Safety {
	return tool.Safety{SideEffect: tool.SideEffectNetwork}
}
func (fakeMCPTool) Run(context.Context, map[string]any) tool.Result { return tool.Result{} }

// fakeLoader implements mcpToolLoader without a real mcp.Loader, so tests
// can script its Tools()/Errs() results directly.
type fakeLoader struct {
	tools    []tool.Tool
	timedOut bool
	errs     map[string]error
}

func (f *fakeLoader) Tools(context.Context, time.Duration) ([]tool.Tool, bool) {
	return f.tools, f.timedOut
}
func (f *fakeLoader) Errs() map[string]error { return f.errs }

// TestRunHeadlessMergesMCPToolsBeforeTheTurn covers issue #48's "registered
// into liam's toolset as if they were built-in Tools" criterion reaching
// the actual running harness: a tool the loader reports must be callable
// by the model on the turn that follows.
func TestRunHeadlessMergesMCPToolsBeforeTheTurn(t *testing.T) {
	ft := fakeMCPTool{name: "mcp_tool"}
	fp := &fakeProviderCallingTool{toolName: "mcp_tool"}
	loop := agent.Loop{Provider: fp, Tools: tool.NewRegistry()}
	loader := &fakeLoader{tools: []tool.Tool{ft}}

	var stdout, stderr bytes.Buffer
	code := runHeadless(loop, loader, config.Config{}, "hi", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runHeadless() = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mcp_tool") {
		t.Errorf("stdout = %q, want a tool-call line for mcp_tool (proving it reached the registry)", stdout.String())
	}
}

// TestRunHeadlessWarnsOnMCPLoadTimeout covers the "logging a warning on
// timeout" criterion.
func TestRunHeadlessWarnsOnMCPLoadTimeout(t *testing.T) {
	loop := agent.Loop{Provider: doneProvider{}, Tools: tool.NewRegistry()}
	loader := &fakeLoader{timedOut: true}

	var stdout, stderr bytes.Buffer
	code := runHeadless(loop, loader, config.Config{}, "hi", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runHeadless() = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "timed out") {
		t.Errorf("stderr = %q, want a timeout warning", stderr.String())
	}
}

// TestRunHeadlessWarnsOnMCPServerError covers a per-server load failure
// (a connect/handshake/list-tools error, independent of timeout) reaching
// the user.
func TestRunHeadlessWarnsOnMCPServerError(t *testing.T) {
	loop := agent.Loop{Provider: doneProvider{}, Tools: tool.NewRegistry()}
	loader := &fakeLoader{errs: map[string]error{"bad-server": errors.New("connect refused")}}

	var stdout, stderr bytes.Buffer
	code := runHeadless(loop, loader, config.Config{}, "hi", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runHeadless() = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "bad-server") || !strings.Contains(stderr.String(), "connect refused") {
		t.Errorf("stderr = %q, want a mention of the failing server and its error", stderr.String())
	}
}

// TestRunHeadlessNilLoaderIsNoOp covers the "no mcpServers configured"
// case: a nil loader must not panic or alter behavior.
func TestRunHeadlessNilLoaderIsNoOp(t *testing.T) {
	loop := agent.Loop{Provider: doneProvider{}, Tools: tool.NewRegistry()}

	var stdout, stderr bytes.Buffer
	code := runHeadless(loop, nil, config.Config{}, "hi", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runHeadless() = %d, want 0; stderr = %q", code, stderr.String())
	}
}

// fakeProviderCallingTool scripts exactly one turn that calls toolName,
// then a final turn with no tool calls — enough to prove a tool actually
// reached the registry the provider's tool list (and dispatch) sees.
type fakeProviderCallingTool struct {
	toolName string
	calls    int
}

func (f *fakeProviderCallingTool) Name() string { return "fake" }
func (f *fakeProviderCallingTool) Stream(_ context.Context, _ provider.Request) iter.Seq2[provider.Event, error] {
	idx := f.calls
	f.calls++
	return func(yield func(provider.Event, error) bool) {
		if idx == 0 {
			if !yield(provider.ToolCallEvent{ID: "call_1", Name: f.toolName, ArgsJSON: `{}`}, nil) {
				return
			}
			yield(provider.DoneEvent{FinishReason: "tool_calls"}, nil)
			return
		}
		yield(provider.DoneEvent{FinishReason: "stop"}, nil)
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

// TestJoinPromptCombinesGeneralToSpecific covers run()'s merge of issue
// #56's discovered project instructions (general) with a -skill
// force-activated body (specific to one headless invocation), and the
// edge cases where either side is empty.
func TestJoinPromptCombinesGeneralToSpecific(t *testing.T) {
	tests := []struct {
		name           string
		project, skill string
		want           string
	}{
		{"both empty", "", "", ""},
		{"project only", "project instructions", "", "project instructions"},
		{"skill only", "", "skill instructions", "skill instructions"},
		{"both present", "project instructions", "skill instructions", "project instructions\n\nskill instructions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinPrompt(tt.project, tt.skill); got != tt.want {
				t.Errorf("joinPrompt(%q, %q) = %q, want %q", tt.project, tt.skill, got, tt.want)
			}
		})
	}
}

// TestBaseSystemPromptPrependsIdentityPreamble covers issue #95's headline
// ordering requirement: liam's fixed identity preamble comes first, ahead
// of discovered project instructions, separated by the same blank-line
// convention joinPrompt uses elsewhere.
func TestBaseSystemPromptPrependsIdentityPreamble(t *testing.T) {
	got := baseSystemPrompt("project instructions")
	want := instructions.Preamble + "\n\nproject instructions"
	if got != want {
		t.Errorf("baseSystemPrompt() = %q, want %q", got, want)
	}
}

// TestBaseSystemPromptPresentWithoutProjectInstructions covers the "no
// AGENTS.md/LIAM.md anywhere" case: the preamble must still be sent in
// full, unlike Load()'s own empty-string result in that case.
func TestBaseSystemPromptPresentWithoutProjectInstructions(t *testing.T) {
	got := baseSystemPrompt("")
	if got != instructions.Preamble {
		t.Errorf("baseSystemPrompt(\"\") = %q, want just the identity preamble", got)
	}
}

// TestBaseSystemPromptSurvivesLargeProjectInstructions guards the preamble
// against being crowded out by instructions.Load()'s own 32 KiB total cap:
// baseSystemPrompt composes after Load returns, so a capped (or otherwise
// large) project-instructions string must not truncate the preamble that
// precedes it.
func TestBaseSystemPromptSurvivesLargeProjectInstructions(t *testing.T) {
	large := strings.Repeat("x", 40*1024)
	got := baseSystemPrompt(large)
	if !strings.HasPrefix(got, instructions.Preamble+"\n\n") {
		t.Error("baseSystemPrompt() with large project instructions did not keep the identity preamble intact and first")
	}
}

// TestBaseSystemPromptSurvivesInstructionsLoadTotalCap is issue #95's AC2
// exercised end-to-end through the real instructions.Load(), not just a
// synthetic string: a set of project instruction files large enough to hit
// Load's own 32 KiB total cap must not crowd out or truncate the identity
// preamble composed in ahead of it by baseSystemPrompt. The fixture mirrors
// internal/instructions's own TestLoadEnforcesTotalCap: four 10 KiB files
// (each truncated to Load's 8 KiB per-file cap first) comfortably exceed
// the 32 KiB total cap once assembled.
func TestBaseSystemPromptSurvivesInstructionsLoadTotalCap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	xdgConfigHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	writeBig := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(strings.Repeat("x", 10*1024)), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	writeBig(filepath.Join(xdgConfigHome, "liam", "LIAM.md"))

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("Mkdir .git: %v", err)
	}
	writeBig(filepath.Join(root, "AGENTS.md"))
	writeBig(filepath.Join(root, "LIAM.md"))
	sub := filepath.Join(root, "sub")
	writeBig(filepath.Join(sub, "AGENTS.md"))

	loaded, err := instructions.Load(sub)
	if err != nil {
		t.Fatalf("instructions.Load: %v", err)
	}
	const totalCap = 32 * 1024 // instructions.go's unexported maxTotalBytes
	if len(loaded) != totalCap {
		t.Fatalf("len(instructions.Load()) = %d, want %d (Load's total cap) — test fixture assumption broke", len(loaded), totalCap)
	}

	got := baseSystemPrompt(loaded)
	if !strings.HasPrefix(got, instructions.Preamble+"\n\n") {
		t.Error("baseSystemPrompt() did not keep the identity preamble intact and first against a Load() result that hit the 32 KiB total cap")
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
	code := runHeadless(loop, nil, config.Config{}, "hi", "", &stdout, &stderr)
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
