package xdg

import (
	"path/filepath"
	"testing"
)

func TestStateHomeUsesEnvVarWhenSet(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	got, err := StateHome()
	if err != nil {
		t.Fatalf("StateHome() error = %v", err)
	}
	if got != "/custom/state" {
		t.Errorf("StateHome() = %q, want %q", got, "/custom/state")
	}
}

func TestStateHomeFallsBackToDotLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := StateHome()
	if err != nil {
		t.Fatalf("StateHome() error = %v", err)
	}
	want := filepath.Join(home, ".local", "state")
	if got != want {
		t.Errorf("StateHome() = %q, want %q", got, want)
	}
}
