// Package config loads liam's JSONC configuration, deep-merging global and
// project files with LIAM_* environment variables and CLI flags layered on
// top, in that order of increasing precedence.
package config

// Config is the fully merged configuration container.
type Config struct {
	Provider   ProviderConfig   `json:"provider"`
	Theme      ThemeConfig      `json:"theme"`
	Hooks      HooksConfig      `json:"hooks"`
	MCPServers MCPServersConfig `json:"mcpServers"`
	Skills     SkillsConfig     `json:"skills"`
	Plugins    PluginsConfig    `json:"plugins"`
	StatusLine StatusLineConfig `json:"statusLine"`
}

// ProviderConfig configures the model provider.
type ProviderConfig struct {
	// Model is an opaque passthrough to provider.Request.Model, e.g.
	// "openrouter/auto". Empty means "let the provider pick its default".
	Model string `json:"model,omitempty"`
}

// ThemeConfig configures theme selection.
type ThemeConfig struct {
	// Mode overrides theme auto-detection: "auto" (default, detected at
	// startup via the terminal's background color), "dark" (Catppuccin
	// Frappe), or "light" (Catppuccin Latte).
	Mode string `json:"mode,omitempty"`
}

// HooksConfig configures liam's 4 hook lifecycle points: sessionStart,
// sessionEnd, beforeTool, afterTool. Each is an array of HookConfig entries,
// run in declaration order.
type HooksConfig struct {
	SessionStart []HookConfig `json:"sessionStart,omitempty"`
	SessionEnd   []HookConfig `json:"sessionEnd,omitempty"`
	BeforeTool   []HookConfig `json:"beforeTool,omitempty"`
	AfterTool    []HookConfig `json:"afterTool,omitempty"`
}

// HookConfig is one hook entry: a shell command run at its lifecycle point.
type HookConfig struct {
	// Command is run via "sh -c". Required.
	Command string `json:"command"`
	// Match restricts a beforeTool/afterTool hook to the named tools;
	// "*" or an empty Match matches every tool. Ignored by sessionStart/
	// sessionEnd, which have no associated tool.
	Match []string `json:"match,omitempty"`
	// TimeoutMs bounds how long the hook process may run before it's
	// killed and treated as a fail-open timeout (ADR-0002). 0 means no
	// timeout.
	TimeoutMs int `json:"timeoutMs,omitempty"`
	// Async fires the hook without waiting for it to finish, so it can
	// never block the agent loop (or, for beforeTool, gate the call it
	// wraps).
	Async bool `json:"async,omitempty"`
}

// MCPServersConfig is a stub, populated by a later ticket.
type MCPServersConfig struct{}

// SkillsConfig configures Agent Skills discovery and trust.
type SkillsConfig struct {
	// Paths are additional directories to scan for skills, beyond the
	// four spec'd default locations (.agents/skills and .liam/skills at
	// both project and user scope). Scanned unconditionally — not
	// subject to the project trust gate, since listing one here is
	// itself an explicit user opt-in.
	Paths []string `json:"paths,omitempty"`
	// Disabled lists skill names to exclude entirely, regardless of
	// where they were discovered.
	Disabled []string `json:"disabled,omitempty"`
	// TrustProjectSkills overrides the one-time interactive per-project
	// trust prompt: true always loads project-scope skills without
	// prompting, false never does. nil (the default) prompts
	// interactively when possible, falling back to false — headless
	// mode's safe "don't load untrusted project skills" default, since
	// blocking on stdin mid-script isn't appropriate there.
	TrustProjectSkills *bool `json:"trustProjectSkills,omitempty"`
}

// PluginsConfig is a stub, populated by a later ticket.
type PluginsConfig struct{}

// StatusLineConfig is a stub, populated by a later ticket.
type StatusLineConfig struct{}
