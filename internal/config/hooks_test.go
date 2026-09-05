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
    "afterTool": [{ "match": ["*"], "command": "./log.sh", "timeoutMs": 5000, "async": true }],
    "agentDone": [{ "command": "./record.sh", "async": true }]
  }
}`)

	cfg, err := Load(cwd, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Hooks.SessionStart) != 1 || cfg.Hooks.SessionStart[0].Command != "echo starting" {
		t.Errorf("Hooks.SessionStart = %+v, want one hook running %q", cfg.Hooks.SessionStart, "echo starting")
	}

	if len(cfg.Hooks.AfterTool) != 1 {
		t.Fatalf("Hooks.AfterTool = %+v, want 1 entry", cfg.Hooks.AfterTool)
	}
	at := cfg.Hooks.AfterTool[0]
	if at.Command != "./log.sh" || at.TimeoutMs != 5000 || !at.Async || len(at.Match) != 1 || at.Match[0] != "*" {
		t.Errorf("Hooks.AfterTool[0] = %+v, want command=./log.sh timeoutMs=5000 async=true match=[*]", at)
	}

	if len(cfg.Hooks.SessionEnd) != 0 {
		t.Errorf("Hooks.SessionEnd = %+v, want none configured", cfg.Hooks.SessionEnd)
	}

	if len(cfg.Hooks.AgentDone) != 1 {
		t.Fatalf("Hooks.AgentDone = %+v, want 1 entry", cfg.Hooks.AgentDone)
	}
	ad := cfg.Hooks.AgentDone[0]
	if ad.Command != "./record.sh" || !ad.Async {
		t.Errorf("Hooks.AgentDone[0] = %+v, want command=./record.sh async=true", ad)
	}
}
