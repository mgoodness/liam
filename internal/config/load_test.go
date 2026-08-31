package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// setupXDG points XDG_CONFIG_HOME at a fresh temp dir and returns it.
func setupXDG(t *testing.T) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	return xdg
}

// TestLoadPrecedence covers the full precedence chain — built-in default <
// global file < project file < LIAM_* env var < CLI flag — table-driven
// over which layers are present for a given test case.
func TestLoadPrecedence(t *testing.T) {
	cases := []struct {
		name      string
		global    string // liam.jsonc content at the global path; "" = absent
		project   string // liam.jsonc content at the project path; "" = absent
		env       string // LIAM_PROVIDER_MODEL value; "" = unset
		flag      string // --model flag value; "" = unset
		wantModel string
	}{
		{
			name:      "no files, no overrides",
			wantModel: "",
		},
		{
			name:      "global only",
			global:    `{"provider": {"model": "openrouter/auto"}}`,
			wantModel: "openrouter/auto",
		},
		{
			name:      "project overrides global",
			global:    `{"provider": {"model": "openrouter/auto"}}`,
			project:   `{"provider": {"model": "openai/gpt-4o"}}`,
			wantModel: "openai/gpt-4o",
		},
		{
			name:      "env overrides files",
			global:    `{"provider": {"model": "openrouter/auto"}}`,
			project:   `{"provider": {"model": "openai/gpt-4o"}}`,
			env:       "anthropic/claude",
			wantModel: "anthropic/claude",
		},
		{
			name:      "flag overrides env and files",
			global:    `{"provider": {"model": "openrouter/auto"}}`,
			project:   `{"provider": {"model": "openai/gpt-4o"}}`,
			env:       "anthropic/claude",
			flag:      "mistral/large",
			wantModel: "mistral/large",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xdg := setupXDG(t)
			if tc.global != "" {
				writeFile(t, filepath.Join(xdg, "liam", "liam.jsonc"), tc.global)
			}

			cwd := t.TempDir()
			if tc.project != "" {
				writeFile(t, filepath.Join(cwd, "liam.jsonc"), tc.project)
			}

			if tc.env != "" {
				t.Setenv("LIAM_PROVIDER_MODEL", tc.env)
			}

			cfg, err := Load(cwd, tc.flag)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Provider.Model != tc.wantModel {
				t.Errorf("Provider.Model = %q, want %q", cfg.Provider.Model, tc.wantModel)
			}
		})
	}
}

func TestLoadProjectConfigDiscoveredByWalkingUp(t *testing.T) {
	setupXDG(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "liam.jsonc"), `{"provider": {"model": "openai/gpt-4o"}}`)

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cfg, err := Load(nested, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider.Model != "openai/gpt-4o" {
		t.Errorf("Provider.Model = %q, want %q", cfg.Provider.Model, "openai/gpt-4o")
	}
}

func TestLoadMissingFilesAreNotErrors(t *testing.T) {
	// XDG_CONFIG_HOME points somewhere with no liam/liam.jsonc, and cwd has
	// no liam.jsonc anywhere up to root — Load should succeed with zero
	// values rather than erroring on the missing files.
	setupXDG(t)
	cwd := t.TempDir()

	if _, err := Load(cwd, ""); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
