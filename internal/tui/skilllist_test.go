package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mgoodness/liam/internal/skill"
	"github.com/mgoodness/liam/internal/theme"
)

func TestRenderSkillListEmpty(t *testing.T) {
	got := ansi.Strip(renderSkillList(theme.Frappe, nil, 80))
	if got != "No skills discovered." {
		t.Errorf("renderSkillList(nil, 80) = %q, want %q", got, "No skills discovered.")
	}
}

func TestRenderSkillListPluralizesHeader(t *testing.T) {
	one := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "a", Description: "d", Scope: skill.ScopeUser},
	}, 80))
	if !strings.HasPrefix(one, "1 skill discovered:") {
		t.Errorf("header = %q, want it to start with %q", one, "1 skill discovered:")
	}

	two := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "a", Description: "d", Scope: skill.ScopeUser},
		{Name: "b", Description: "d", Scope: skill.ScopeUser},
	}, 80))
	if !strings.HasPrefix(two, "2 skills discovered:") {
		t.Errorf("header = %q, want it to start with %q", two, "2 skills discovered:")
	}
	if strings.Contains(two, "(s)") {
		t.Errorf("header = %q, want no literal \"(s)\"", two)
	}
}

func TestRenderSkillListGroupsByScopeMostSpecificFirst(t *testing.T) {
	got := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "user-skill", Description: "d", Scope: skill.ScopeUser},
		{Name: "project-skill", Description: "d", Scope: skill.ScopeProject},
	}, 80))

	projectIdx := strings.Index(got, "Project skills")
	userIdx := strings.Index(got, "User skills")
	if projectIdx == -1 || userIdx == -1 {
		t.Fatalf("output = %q, want both a Project skills and User skills heading", got)
	}
	if projectIdx > userIdx {
		t.Errorf("Project skills heading (%d) came after User skills heading (%d), want project first", projectIdx, userIdx)
	}
	if strings.Contains(got, "Extra skills") {
		t.Errorf("output = %q, want no heading for the empty extra scope", got)
	}
}

func TestRenderSkillListSingleGroupStillGetsHeading(t *testing.T) {
	got := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "a", Description: "d", Scope: skill.ScopeProject},
	}, 80))
	if !strings.Contains(got, "Project skills") {
		t.Errorf("output = %q, want a Project skills heading even with a single group", got)
	}
}

func TestRenderSkillListDisabledSkillDistinctFromDescriptionText(t *testing.T) {
	styled := renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "hidden", Description: "d", Scope: skill.ScopeProject, DisableModelInvocation: true},
	}, 80)
	if strings.Contains(styled, "[not model-invocable]") {
		t.Errorf("renderSkillList() = %q, want no same-weight inline text suffix", styled)
	}
	stripped := ansi.Strip(styled)
	if !strings.Contains(stripped, "⊘") {
		t.Errorf("renderSkillList() = %q, want a distinct marker for a disabled skill", stripped)
	}
	if styled == stripped {
		t.Errorf("renderSkillList() carried no ANSI styling at all, want the disabled row visually distinguished")
	}
}

func TestRenderSkillListAlignsDescriptionColumnWithinGroup(t *testing.T) {
	got := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "a", Description: "short name", Scope: skill.ScopeUser},
		{Name: "much-longer-skill-name", Description: "long name", Scope: skill.ScopeUser},
	}, 80))

	var dashCols []int
	for _, line := range strings.Split(got, "\n") {
		if i := strings.Index(line, "—"); i >= 0 {
			dashCols = append(dashCols, len([]rune(line[:i])))
		}
	}
	if len(dashCols) != 2 {
		t.Fatalf("output = %q, want 2 rows with a description column", got)
	}
	if dashCols[0] != dashCols[1] {
		t.Errorf("description column starts at %v, want the same column for every row", dashCols)
	}
}

// TestRenderSkillListAlignsColumnsPerGroupNotGlobally covers the AC that
// column alignment/truncation is preserved "within each scope group" — a
// long name in one group must not widen another group's column.
func TestRenderSkillListAlignsColumnsPerGroupNotGlobally(t *testing.T) {
	got := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "a", Description: "short-group desc", Scope: skill.ScopeUser},
		{Name: strings.Repeat("x", 40), Description: "long-group desc", Scope: skill.ScopeProject},
	}, 80))

	var userDash, projectDash int
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "  a") {
			userDash = strings.Index(line, "—")
		}
		if strings.HasPrefix(line, "  "+strings.Repeat("x", 40)) {
			projectDash = strings.Index(line, "—")
		}
	}
	if userDash == 0 || projectDash == 0 {
		t.Fatalf("output = %q, want to find both rows", got)
	}
	if userDash >= projectDash {
		t.Errorf("user group's description column (%d) was widened by the project group's longer name (%d), want it to stay narrow", userDash, projectDash)
	}
}

func TestRenderSkillListTruncatesLongDescriptionToWidth(t *testing.T) {
	got := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "a", Description: strings.Repeat("x", 200), Scope: skill.ScopeUser},
	}, 40))

	for _, line := range strings.Split(got, "\n") {
		if n := len([]rune(line)); n > 40 {
			t.Errorf("line = %q, width = %d runes, want <= 40 (width)", line, n)
		}
	}
	if !strings.Contains(got, "…") {
		t.Errorf("output = %q, want a hard-truncated ellipsis", got)
	}
}

func TestRenderSkillListNoWidthSkipsTruncation(t *testing.T) {
	desc := strings.Repeat("x", 200)
	got := ansi.Strip(renderSkillList(theme.Frappe, []skill.Skill{
		{Name: "a", Description: desc, Scope: skill.ScopeUser},
	}, 0))

	if !strings.Contains(got, desc) {
		t.Errorf("renderSkillList(..., 0) truncated the description, want it left untouched")
	}
}
