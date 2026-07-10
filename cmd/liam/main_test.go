package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgoodness/liam/skill"
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

func TestAppendSkillIndex_NoEligibleSkillsReturnsBaseUnchanged(t *testing.T) {
	got := appendSkillIndex("base prompt", nil)
	if got != "base prompt" {
		t.Errorf("appendSkillIndex = %q, want unchanged base prompt", got)
	}
}

func TestAppendSkillIndex_AppendsIndexForEligibleSkill(t *testing.T) {
	skills := []skill.Skill{
		{Name: "pdf-processing", Description: "Handles PDFs.", Path: "/skills/pdf-processing"},
	}

	got := appendSkillIndex("base prompt", skills)
	if !strings.HasPrefix(got, "base prompt\n\n") {
		t.Errorf("appendSkillIndex = %q, want it to start with the base prompt followed by a blank line", got)
	}
	if !strings.Contains(got, "pdf-processing") {
		t.Errorf("appendSkillIndex = %q, want it to contain the skill index", got)
	}
}
