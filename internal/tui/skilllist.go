package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/render"
	"github.com/mgoodness/liam/internal/skill"
	"github.com/mgoodness/liam/internal/theme"
)

// skillListPrefix and skillListDisabledPrefix open every /skills row —
// both 2 runes wide so a disabled row's leading "⊘" marker doesn't shift
// its description column relative to an enabled row's.
const (
	skillListPrefix         = "  "
	skillListDisabledPrefix = "⊘ "
)

// skillScopeHeading titles a scope's group heading in skillGroupOrder's
// display order (most specific first).
var skillScopeHeading = map[skill.Scope]string{
	skill.ScopeProject: "Project skills",
	skill.ScopeUser:    "User skills",
	skill.ScopeExtra:   "Extra skills",
}

// renderSkillList renders liam's discovered skill catalog for the /skills
// command (issue #82): a pluralized count header, then one heading and
// table per non-empty scope group from render.SkillGroups (project, user,
// extra — most specific first), name/description columns aligned and
// truncated to width — independently per group, so one group's longest
// name never widens another group's column — via the same
// render.TableColumns/render.FitLabelDesc machinery renderSlashPopup's
// table already relies on (issue #148). A skill with disable-model-
// invocation set gets a leading "⊘" marker and the whole row dimmed — a
// distinct visual treatment rather than the plain inline "[not
// model-invocable]" suffix the old flat layout used, per issue #82's
// brief — while keeping the prefix's rune width identical to an enabled
// row's so the description column still lines up. width <= 0 disables
// truncation and label-column capping, matching render.TableColumns'
// "headless caller" convention.
func renderSkillList(p theme.Palette, skills []skill.Skill, width int) string {
	if len(skills) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Italic(true).Render("No skills discovered.")
	}

	heading := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Mauve)).Bold(true)
	name := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Bold(true)
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext))
	disabled := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Italic(true)

	overhead := len([]rune(skillListPrefix)) + len([]rune(slashColumnSeparator))

	var b strings.Builder
	fmt.Fprintf(&b, "%d %s discovered:", len(skills), render.Pluralize(len(skills), "skill", "skills"))

	for _, g := range render.SkillGroups(skills) {
		b.WriteString("\n\n")
		b.WriteString(heading.Render(skillScopeHeading[g.Scope]))

		names := make([]string, len(g.Skills))
		for i, s := range g.Skills {
			names[i] = s.Name
		}
		var nameWidth, descWidth int
		if width > 0 {
			nameWidth, descWidth = render.TableColumns(names, width, overhead, render.MinDescWidth)
		} else {
			nameWidth = render.ColumnWidth(names, 0)
		}

		for _, s := range g.Skills {
			b.WriteByte('\n')
			n, d := render.FitLabelDesc(s.Name, s.Description, nameWidth, descWidth, width)
			padded := fmt.Sprintf("%-*s", nameWidth, n)

			if s.DisableModelInvocation {
				b.WriteString(disabled.Render(skillListDisabledPrefix + padded + descriptionSuffix(d)))
				continue
			}
			b.WriteString(name.Render(skillListPrefix + padded))
			b.WriteString(desc.Render(descriptionSuffix(d)))
		}
	}
	return b.String()
}
