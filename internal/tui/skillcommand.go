package tui

import (
	"strings"

	"github.com/mgoodness/liam/internal/skill"
)

// skillCommandName reports the skill name a bare "/<name>" input would
// activate, plus any trailing text after it — ticket #16's decided
// force-activation path, typed the same way Claude Code itself invokes a
// skill (a bare slash command, no separate "skill" keyword). rest is ""
// if nothing follows the name, in which case the caller starts no turn;
// otherwise it's non-empty and the caller starts a turn immediately,
// mirroring how Claude Code's own slash commands pass arguments through
// and act on them right away rather than requiring a separate follow-up
// message. Either way the caller submits the full literal command text
// (not just rest) as the user-visible/sent message — rest's only job here
// is signaling whether to start a turn.
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
// flag already uses to force-activate a skill for an entire run. It
// renders no confirmation line of its own and starts no turn — the caller
// (submit) prints the literal command like any other input and decides
// whether trailing text after the skill name means a turn should start
// immediately.
func (m *Model) activateSkill(s skill.Skill) {
	if m.systemPrompt == "" {
		m.systemPrompt = s.Body
	} else {
		m.systemPrompt = m.systemPrompt + "\n\n" + s.Body
	}
}
