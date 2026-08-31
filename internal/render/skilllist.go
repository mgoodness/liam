package render

import (
	"fmt"
	"strings"

	"github.com/mgoodness/liam/internal/skill"
)

// SkillList formats a discovered skill catalog for the /skills command:
// one "name (scope) — description" line per skill, sorted the same way
// skill.Discover already returns them (by name). A skill with
// disable-model-invocation: true — excluded from the model-driven
// activate_skill catalog — is marked accordingly, since it's still
// force-activatable directly.
func SkillList(skills []skill.Skill) string {
	if len(skills) == 0 {
		return "No skills discovered."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d skill(s) discovered:\n", len(skills))
	for _, s := range skills {
		fmt.Fprintf(&b, "  %s (%s) — %s", s.Name, s.Scope, s.Description)
		if s.DisableModelInvocation {
			b.WriteString(" [not model-invocable]")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
