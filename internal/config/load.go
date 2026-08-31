package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load discovers global and project liam.jsonc files starting from cwd,
// deep-merges them (project overrides global on conflicting keys), then
// layers LIAM_* environment variables and finally flagModel (the value of
// a --model CLI flag, "" if unset) on top, in that order of increasing
// precedence. A missing config file at either location is not an error.
func Load(cwd, flagModel string) (Config, error) {
	merged := map[string]any{}

	globalPath, err := globalConfigPath()
	if err != nil {
		return Config{}, fmt.Errorf("config: locating global config: %w", err)
	}
	if globalPath != "" {
		m, err := readJSONCFile(globalPath)
		if err != nil {
			return Config{}, err
		}
		merged = mergeMaps(merged, m)
	}

	projectPath, err := findProjectConfig(cwd)
	if err != nil {
		return Config{}, fmt.Errorf("config: locating project config: %w", err)
	}
	if projectPath != "" {
		m, err := readJSONCFile(projectPath)
		if err != nil {
			return Config{}, err
		}
		merged = mergeMaps(merged, m)
	}

	var cfg Config
	if len(merged) > 0 {
		b, err := json.Marshal(merged)
		if err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
	}

	applyEnv(&cfg)

	if flagModel != "" {
		cfg.Provider.Model = flagModel
	}

	return cfg, nil
}

// readJSONCFile reads path, strips JSONC comments, and unmarshals it into a
// generic map for merging.
func readJSONCFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(stripComments(raw), &m); err != nil {
		return nil, fmt.Errorf("config: parsing %s: %w", path, err)
	}
	return m, nil
}

// applyEnv layers LIAM_* environment variables onto cfg, above file config
// but below any CLI flag override applied by the caller.
func applyEnv(cfg *Config) {
	if v := os.Getenv("LIAM_PROVIDER_MODEL"); v != "" {
		cfg.Provider.Model = v
	}
}
