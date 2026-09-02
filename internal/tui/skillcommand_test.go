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
// model to call activate_skill on its own. Activation itself renders no
// confirmation line, but the command the user typed is still printed like
// any other submitted input.
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
	if len(mm.lines) != 1 || mm.lines[0].role != "user" || mm.lines[0].text != "/implement" {
		t.Errorf("lines = %+v, want a single user line reading \"/implement\" — no confirmation line, but the command itself printed", mm.lines)
	}
}

// TestSubmitBareSlashNameWithTrailingTextStartsATurn is a regression test:
// "/implement #123" previously activated the skill and printed a
// confirmation line, but nothing else happened — the trailing text was
// silently discarded instead of being sent as the first turn's message.
// Trailing text after the skill name now starts a turn immediately,
// mirroring how Claude Code's own slash commands pass arguments through —
// and the full literal command is what's printed/sent, exactly as typed,
// the same as any other submitted input.
func TestSubmitBareSlashNameWithTrailingTextStartsATurn(t *testing.T) {
	fp := &capturingProvider{}
	skills := []skill.Skill{
		{Name: "implement", Description: "Implement a feature.", Body: "IMPLEMENT SKILL BODY"},
	}
	m := New(agent.Loop{Provider: fp}, config.Config{}, skills)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	m.input.SetValue("/implement #123")

	next, cmd := m.submit()
	if cmd == nil {
		t.Fatal("submit(\"/implement #123\") returned a nil cmd, want a turn to start")
	}
	mm := drain(t, next.(Model), cmd)

	if !strings.Contains(mm.systemPrompt, "IMPLEMENT SKILL BODY") {
		t.Errorf("systemPrompt = %q, want it to contain the activated skill's body", mm.systemPrompt)
	}
	if fp.lastReq.Messages == nil || fp.lastReq.Messages[len(fp.lastReq.Messages)-1].Content != "/implement #123" {
		t.Fatalf("lastReq.Messages = %+v, want the full literal command \"/implement #123\" sent as the last message", fp.lastReq.Messages)
	}
	if len(mm.lines) != 2 || mm.lines[0].role != "user" || mm.lines[0].text != "/implement #123" {
		t.Errorf("lines = %+v, want a user line reading \"/implement #123\" (no confirmation line, the literal command printed like any other input) followed by the completion line", mm.lines)
	}
	if mm.lines[1].role != "complete" {
		t.Errorf("lines[1].role = %q, want %q", mm.lines[1].role, "complete")
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
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
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
