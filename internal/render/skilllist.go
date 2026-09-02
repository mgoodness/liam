package render

import (
	"fmt"
	"strings"

	"github.com/mgoodness/liam/internal/skill"
)

// SkillList formats a discovered skill catalog for the /skills command as
// a two-column table — "name (scope)" padded to a shared width, then
// " — description" — sorted the same way skill.Discover already returns
// them (by name). width is the terminal width to align and truncate the
// table against (issue #148); width <= 0 (e.g. a headless caller with no
// terminal) disables both truncation and label-column capping entirely,
// matching statusline.truncateRows' convention. A skill with
// disable-model-invocation: true — excluded from the model-driven
// activate_skill catalog — is marked accordingly, since it's still
// force-activatable directly.
func SkillList(skills []skill.Skill, width int) string {
	if len(skills) == 0 {
		return "No skills discovered."
	}

	const prefix, separator = "  ", " — "
	overhead := len([]rune(prefix)) + len([]rune(separator))

	labels := make([]string, len(skills))
	for i, s := range skills {
		labels[i] = fmt.Sprintf("%s (%s)", s.Name, s.Scope)
	}

	var labelWidth, descWidth int
	if width > 0 {
		labelWidth, descWidth = TableColumns(labels, width, overhead, MinDescWidth)
	} else {
		labelWidth = ColumnWidth(labels, 0)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d skill(s) discovered:\n", len(skills))
	for i, s := range skills {
		label := labels[i]
		desc := s.Description
		if s.DisableModelInvocation {
			desc += " [not model-invocable]"
		}
		if width > 0 {
			if l := len([]rune(label)); l > labelWidth {
				label = TruncateWidth(label, labelWidth)
			}
			desc = TruncateWidth(desc, descWidth)
		}
		fmt.Fprintf(&b, "%s%-*s%s%s\n", prefix, labelWidth, label, separator, desc)
	}
	return strings.TrimRight(b.String(), "\n")
}
