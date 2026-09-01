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

// MCPServersConfig configures liam's MCP client, keyed by server name — the
// convention shared with Claude Desktop, Claude Code, and Cursor.
type MCPServersConfig map[string]MCPServerConfig

// MCPServerConfig is one stdio-transport MCP server: launched as a
// subprocess, its tools capability registered into liam's toolset.
type MCPServerConfig struct {
	// Command is the executable to launch. Required.
	Command string `json:"command"`
	// Args are passed to Command.
	Args []string `json:"args,omitempty"`
	// Env sets additional environment variables for the subprocess. Values
	// support $VAR expansion against the harness's own environment.
	Env map[string]string `json:"env,omitempty"`
	// Tools, if non-empty, allow-lists which of the server's tools are
	// registered into liam's toolset, protecting the context budget from a
	// large server. Empty registers every tool the server exposes.
	Tools []string `json:"tools,omitempty"`
}

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

// StatusLineConfig configures the customizable status block pinned below
// the input line (the interactive screen's bottom footer): an
// external-command hook, the same shape as HooksConfig, modeled directly
// on Claude Code's own statusLine primitive.
type StatusLineConfig struct {
	// Command, when set, is run via "sh -c" and receives session JSON on
	// stdin; each line it prints to stdout becomes one status-block row.
	// Unset (the default) uses the built-in renderer (identity line +
	// metrics bar).
	Command string `json:"command,omitempty"`
	// RefreshInterval adds a periodic timer refresh, in milliseconds,
	// alongside the status block's other refresh triggers (session start,
	// after each response, after each tool call) — every trigger,
	// including this one, is debounced at 300ms. 0 (the default) disables
	// the timer; a configured positive value below the 1s floor is raised
	// to it (see statusline.RefreshInterval).
	RefreshInterval int `json:"refreshInterval,omitempty"`
}
