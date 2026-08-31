package config

import "testing"

// TestLoadParsesHooksConfig covers issue #45's config surface: the hooks
// section's 4 lifecycle-point arrays, each entry's command/match/timeoutMs/
// async fields, loaded the same way Load loads everything else.
func TestLoadParsesHooksConfig(t *testing.T) {
	setupXDG(t)
	cwd := t.TempDir()
	writeFile(t, cwd+"/liam.jsonc", `{
  "hooks": {
    "sessionStart": [{ "command": "echo starting" }],
    "beforeTool": [{ "match": ["bash", "edit"], "command": "./check.sh", "timeoutMs": 5000 }],
    "afterTool": [{ "match": ["*"], "command": "./log.sh", "async": true }]
  }
}`)

	cfg, err := Load(cwd, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Hooks.SessionStart) != 1 || cfg.Hooks.SessionStart[0].Command != "echo starting" {
		t.Errorf("Hooks.SessionStart = %+v, want one hook running %q", cfg.Hooks.SessionStart, "echo starting")
	}

	if len(cfg.Hooks.BeforeTool) != 1 {
		t.Fatalf("Hooks.BeforeTool = %+v, want 1 entry", cfg.Hooks.BeforeTool)
	}
	bt := cfg.Hooks.BeforeTool[0]
	if bt.Command != "./check.sh" || bt.TimeoutMs != 5000 || bt.Async {
		t.Errorf("Hooks.BeforeTool[0] = %+v, want command=./check.sh timeoutMs=5000 async=false", bt)
	}
	if len(bt.Match) != 2 || bt.Match[0] != "bash" || bt.Match[1] != "edit" {
		t.Errorf("Hooks.BeforeTool[0].Match = %+v, want [bash edit]", bt.Match)
	}

	if len(cfg.Hooks.AfterTool) != 1 {
		t.Fatalf("Hooks.AfterTool = %+v, want 1 entry", cfg.Hooks.AfterTool)
	}
	at := cfg.Hooks.AfterTool[0]
	if at.Command != "./log.sh" || !at.Async || len(at.Match) != 1 || at.Match[0] != "*" {
		t.Errorf("Hooks.AfterTool[0] = %+v, want command=./log.sh async=true match=[*]", at)
	}

	if len(cfg.Hooks.SessionEnd) != 0 {
		t.Errorf("Hooks.SessionEnd = %+v, want none configured", cfg.Hooks.SessionEnd)
	}
}
