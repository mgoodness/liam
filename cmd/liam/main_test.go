package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullSystemPrompt_AppendsDiscoveredAgentsMD(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := t.TempDir()
	initGitRepo(t, dir)
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "project instructions")
	t.Chdir(dir)

	var errOut bytes.Buffer
	got := fullSystemPrompt(&errOut)

	if !strings.HasPrefix(got, systemPrompt) {
		t.Errorf("expected base systemPrompt as a prefix, got %q", got)
	}
	if !strings.Contains(got, "project instructions") {
		t.Errorf("expected discovered AGENTS.md content appended, got %q", got)
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr output: %q", errOut.String())
	}
}

func TestFullSystemPrompt_NoAgentsMDReturnsBasePromptUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)

	var errOut bytes.Buffer
	got := fullSystemPrompt(&errOut)

	if got != systemPrompt {
		t.Errorf("got %q, want systemPrompt unchanged", got)
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr output: %q", errOut.String())
	}
}
