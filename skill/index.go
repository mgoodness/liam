package skill

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SkillMDPath returns the path to the skill's SKILL.md file — what the
// model calls the existing read tool against to load a skill's full body.
func (s Skill) SkillMDPath() string {
	return filepath.Join(s.Path, "SKILL.md")
}

// Index renders the model-facing skill index for injection into the
// system prompt: name, path, and description for every skill not marked
// DisableModelInvocation, one per line in skills' given order. A model
// deciding a task matches a skill's description loads it by calling the
// existing read tool against the listed path — there is no dedicated
// "load skill" tool. Returns "" if no skill is eligible.
func Index(skills []Skill) string {
	var b strings.Builder
	for _, s := range skills {
		if s.DisableModelInvocation {
			continue
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", s.Name, s.SkillMDPath(), s.Description)
	}
	if b.Len() == 0 {
		return ""
	}
	return "Skills available — call read on a skill's path to load its full instructions when its description matches the task:\n" + b.String()
}
