// Package provider implements the Copilot Provider: device-flow login,
// credential storage, and (in later tickets) the chat-completion API.
package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Credentials holds the GitHub OAuth token obtained via device-flow login
// and the short-lived Copilot token exchanged from it.
//
// See CONTEXT.md for why these are two distinct tokens.
type Credentials struct {
	GitHubToken        string    `json:"github_token"`
	CopilotToken       string    `json:"copilot_token"`
	CopilotTokenExpiry time.Time `json:"copilot_token_expiry"`
}

func (c *Credentials) copilotTokenValid(now time.Time) bool {
	return c.CopilotToken != "" && now.Before(c.CopilotTokenExpiry)
}

// credentialsPath returns $XDG_CONFIG_HOME/liam/credentials.json, falling
// back to ~/.config/liam/credentials.json when XDG_CONFIG_HOME is unset.
func credentialsPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "liam", "credentials.json"), nil
}

func loadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

func saveCredentials(path string, creds *Credentials) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	// os.WriteFile only applies the given mode when creating a new file, so
	// an overwrite of a pre-existing file (e.g. from a prior liam version)
	// wouldn't otherwise be tightened to 0600.
	return os.Chmod(path, 0o600)
}
