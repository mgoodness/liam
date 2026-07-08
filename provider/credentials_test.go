package provider

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCredentialsPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg-config")

	got, err := credentialsPath()
	if err != nil {
		t.Fatalf("credentialsPath: %v", err)
	}

	want := filepath.Join("/xdg-config", "liam", "credentials.json")
	if got != want {
		t.Errorf("credentialsPath = %q, want %q", got, want)
	}
}

func TestCredentialsPathFallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/testuser")

	got, err := credentialsPath()
	if err != nil {
		t.Fatalf("credentialsPath: %v", err)
	}

	want := filepath.Join("/home/testuser", ".config", "liam", "credentials.json")
	if got != want {
		t.Errorf("credentialsPath = %q, want %q", got, want)
	}
}

func TestSaveAndLoadCredentialsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "credentials.json")
	want := &Credentials{
		GitHubToken:        "gho_test",
		CopilotToken:       "tid=test",
		CopilotTokenExpiry: time.Now().Add(time.Hour).Truncate(time.Second).UTC(),
	}

	if err := saveCredentials(path, want); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}

	got, err := loadCredentials(path)
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}

	if got.GitHubToken != want.GitHubToken || got.CopilotToken != want.CopilotToken || !got.CopilotTokenExpiry.Equal(want.CopilotTokenExpiry) {
		t.Errorf("loadCredentials = %+v, want %+v", got, want)
	}
}

func TestSaveCredentialsWritesMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")

	if err := saveCredentials(path, &Credentials{GitHubToken: "gho_test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials file mode = %o, want 0600", perm)
	}
}

func TestSaveCredentialsTightensPermissionsOnOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := saveCredentials(path, &Credentials{GitHubToken: "gho_test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials file mode after overwrite = %o, want 0600", perm)
	}
}
