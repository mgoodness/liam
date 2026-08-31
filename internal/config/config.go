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

// HooksConfig is a stub, populated by a later ticket.
type HooksConfig struct{}

// MCPServersConfig is a stub, populated by a later ticket.
type MCPServersConfig struct{}

// SkillsConfig is a stub, populated by a later ticket.
type SkillsConfig struct{}

// PluginsConfig is a stub, populated by a later ticket.
type PluginsConfig struct{}

// StatusLineConfig is a stub, populated by a later ticket.
type StatusLineConfig struct{}
