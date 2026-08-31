package config

import "testing"

// TestLoadParsesMCPServersConfig covers issue #48's config surface: the
// mcpServers section, keyed by server name, with command/args/env/tools
// per entry, loaded the same way Load loads everything else.
func TestLoadParsesMCPServersConfig(t *testing.T) {
	setupXDG(t)
	cwd := t.TempDir()
	writeFile(t, cwd+"/liam.jsonc", `{
  "mcpServers": {
    "fff": {
      "command": "fff-mcp",
      "args": ["--stdio"],
      "env": { "FFF_ROOT": "$HOME/code" },
      "tools": ["find_files", "grep"]
    },
    "bare": {
      "command": "bare-mcp"
    }
  }
}`)

	cfg, err := Load(cwd, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.MCPServers) != 2 {
		t.Fatalf("MCPServers = %+v, want 2 entries", cfg.MCPServers)
	}

	fff, ok := cfg.MCPServers["fff"]
	if !ok {
		t.Fatalf("MCPServers[\"fff\"] missing, got %+v", cfg.MCPServers)
	}
	if fff.Command != "fff-mcp" {
		t.Errorf("fff.Command = %q, want fff-mcp", fff.Command)
	}
	if len(fff.Args) != 1 || fff.Args[0] != "--stdio" {
		t.Errorf("fff.Args = %+v, want [--stdio]", fff.Args)
	}
	if fff.Env["FFF_ROOT"] != "$HOME/code" {
		t.Errorf("fff.Env[FFF_ROOT] = %q, want unexpanded %q (expansion happens at launch time)", fff.Env["FFF_ROOT"], "$HOME/code")
	}
	if len(fff.Tools) != 2 || fff.Tools[0] != "find_files" || fff.Tools[1] != "grep" {
		t.Errorf("fff.Tools = %+v, want [find_files grep]", fff.Tools)
	}

	bare, ok := cfg.MCPServers["bare"]
	if !ok {
		t.Fatalf("MCPServers[\"bare\"] missing, got %+v", cfg.MCPServers)
	}
	if bare.Command != "bare-mcp" {
		t.Errorf("bare.Command = %q, want bare-mcp", bare.Command)
	}
	if len(bare.Tools) != 0 {
		t.Errorf("bare.Tools = %+v, want none (no allow-list configured)", bare.Tools)
	}
}
