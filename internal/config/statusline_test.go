package config

import "testing"

// TestLoadParsesStatusLineConfig covers issue #60's config surface:
// statusLine.command and statusLine.refreshInterval, loaded the same way
// Load loads everything else.
func TestLoadParsesStatusLineConfig(t *testing.T) {
	setupXDG(t)
	cwd := t.TempDir()
	writeFile(t, cwd+"/liam.jsonc", `{
  "statusLine": {
    "command": "./my-statusline.sh",
    "refreshInterval": 2000
  }
}`)

	cfg, err := Load(cwd, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.StatusLine.Command != "./my-statusline.sh" {
		t.Errorf("StatusLine.Command = %q, want %q", cfg.StatusLine.Command, "./my-statusline.sh")
	}
	if cfg.StatusLine.RefreshInterval != 2000 {
		t.Errorf("StatusLine.RefreshInterval = %d, want 2000", cfg.StatusLine.RefreshInterval)
	}
}

// TestLoadStatusLineConfigDefaultsToZeroValue covers the unset case: no
// statusLine section at all leaves Command empty and RefreshInterval 0,
// the built-in-renderer/no-timer defaults.
func TestLoadStatusLineConfigDefaultsToZeroValue(t *testing.T) {
	setupXDG(t)
	cwd := t.TempDir()
	writeFile(t, cwd+"/liam.jsonc", `{"provider": {"model": "openrouter/auto"}}`)

	cfg, err := Load(cwd, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.StatusLine.Command != "" || cfg.StatusLine.RefreshInterval != 0 {
		t.Errorf("StatusLine = %+v, want the zero value when unset", cfg.StatusLine)
	}
}
