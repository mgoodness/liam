package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/skill"
)

// skillCommandName reports the skill name a bare "/<name>" input would
// activate — ticket #16's decided force-activation path, typed the same
// way Claude Code itself invokes a skill (a bare slash command, no
// separate "skill" keyword). Only the exact single token after the slash
// is taken as the name; any text after a space is currently discarded
// (skill activation carries no arguments of its own — the user's next
// message is a separate turn).
func skillCommandName(text string) (string, bool) {
	rest, ok := strings.CutPrefix(text, "/")
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(rest, " ")
	return name, name != ""
}

// activateSkill force-activates s, bypassing model judgment entirely
// (unlike activate_skill, which the model calls on its own) by folding its
// body into systemPrompt — the same mechanism headless mode's -skill flag
// already uses to force-activate a skill for an entire run. No turn is
// started, matching /skills' own no-turn convention; the activated skill
// takes effect starting with the next turn the user submits.
func (m Model) activateSkill(s skill.Skill) (tea.Model, tea.Cmd) {
	if m.systemPrompt == "" {
		m.systemPrompt = s.Body
	} else {
		m.systemPrompt = m.systemPrompt + "\n\n" + s.Body
	}
	m.lines = append(m.lines, line{role: "info", text: fmt.Sprintf("Activated skill %q.", s.Name)})
	m.refreshViewport()
	return m, nil
}
