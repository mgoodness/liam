package tui

import (
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/skill"
)

// TestSubmitBareSlashNameForceActivatesSkill covers ticket #16's decided
// (but until now unbuilt) force-activation command, typed the same way
// Claude Code invokes a skill — a bare "/<name>", no separate "skill"
// keyword. It should inject the named skill's body into the conversation
// the same way headless mode's -skill flag does, without requiring the
// model to call activate_skill on its own.
func TestSubmitBareSlashNameForceActivatesSkill(t *testing.T) {
	skills := []skill.Skill{
		{Name: "implement", Description: "Implement a feature.", Body: "IMPLEMENT SKILL BODY"},
	}
	m := New(agent.Loop{}, config.Config{}, skills)
	m.input.SetValue("/implement")

	next, cmd := m.submit()
	mm := next.(Model)

	if cmd != nil {
		t.Error("submit(\"/implement\") returned a non-nil cmd, want nil (no turn started)")
	}
	if !strings.Contains(mm.systemPrompt, "IMPLEMENT SKILL BODY") {
		t.Errorf("systemPrompt = %q, want it to contain the activated skill's body", mm.systemPrompt)
	}
	if len(mm.lines) != 1 || mm.lines[0].role != "info" {
		t.Fatalf("lines = %+v, want a single info line confirming activation", mm.lines)
	}
	if !strings.Contains(mm.lines[0].text, "implement") {
		t.Errorf("lines[0].text = %q, want it to mention the activated skill's name", mm.lines[0].text)
	}
}

// TestSubmitBareSlashNameWithNoMatchingSkillFallsThroughToChat covers a
// bare "/<name>" that doesn't match any discovered skill: liam has no
// fixed slash-command registry, so this isn't distinguishable from an
// ordinary chat message that happens to start with "/" — it must be sent
// to the model like any other input, not dropped or treated as an error.
func TestSubmitBareSlashNameWithNoMatchingSkillFallsThroughToChat(t *testing.T) {
	fp := &capturingProvider{}
	m := New(agent.Loop{Provider: fp}, config.Config{}, nil)
	m.input.SetValue("/nonexistent")

	next, cmd := m.submit()
	mm := drain(t, next.(Model), cmd)

	if fp.lastReq.Messages == nil {
		t.Fatal("model was never called; want \"/nonexistent\" sent through as a chat message")
	}
	if len(mm.lines) == 0 || mm.lines[0].role != "user" || mm.lines[0].text != "/nonexistent" {
		t.Errorf("lines[0] = %+v, want the literal text rendered as a user line", mm.lines[0])
	}
	if mm.systemPrompt != "" {
		t.Errorf("systemPrompt = %q, want unchanged when no skill matches", mm.systemPrompt)
	}
}
