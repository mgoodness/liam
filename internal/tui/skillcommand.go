package tui

import (
	"strings"

	"github.com/mgoodness/liam/internal/skill"
)

// skillCommandName reports the skill name a bare "/<name>" input would
// activate, plus any trailing text after it — ticket #16's decided
// force-activation path, typed the same way Claude Code itself invokes a
// skill (a bare slash command, no separate "skill" keyword). rest is "" if
// nothing follows the name; when non-empty, the caller sends it as the
// activated skill's first turn, mirroring how Claude Code's own slash
// commands pass trailing text through as the command's arguments and act
// on them immediately, rather than requiring a separate follow-up message.
func skillCommandName(text string) (name, rest string, ok bool) {
	body, ok := strings.CutPrefix(text, "/")
	if !ok {
		return "", "", false
	}
	name, rest, _ = strings.Cut(body, " ")
	return name, strings.TrimSpace(rest), name != ""
}

// activateSkill force-activates s, bypassing model judgment entirely
// (unlike activate_skill, which the model calls on its own) by folding its
// body into m.systemPrompt — the same mechanism headless mode's -skill
// flag already uses to force-activate a skill for an entire run. Silent by
// design: it starts no turn and renders no confirmation line itself; the
// caller (submit) decides whether trailing text after the skill name means
// a turn should start immediately, and that turn's own user/assistant
// lines are the only visible sign activation happened.
func (m *Model) activateSkill(s skill.Skill) {
	if m.systemPrompt == "" {
		m.systemPrompt = s.Body
	} else {
		m.systemPrompt = m.systemPrompt + "\n\n" + s.Body
	}
}
