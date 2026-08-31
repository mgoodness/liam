package render

import (
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/skill"
)

func TestSkillListEmpty(t *testing.T) {
	if got := SkillList(nil); got != "No skills discovered." {
		t.Errorf("SkillList(nil) = %q, want %q", got, "No skills discovered.")
	}
}

func TestSkillListFormatsNameScopeDescription(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "commit-messages", Description: "Write conventional commit messages.", Scope: skill.ScopeUser},
	})
	want := "1 skill(s) discovered:\n  commit-messages (user) — Write conventional commit messages."
	if got != want {
		t.Errorf("SkillList() = %q, want %q", got, want)
	}
}

func TestSkillListMarksDisableModelInvocation(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "hidden", Description: "d", Scope: skill.ScopeProject, DisableModelInvocation: true},
	})
	if !strings.Contains(got, "[not model-invocable]") {
		t.Errorf("SkillList() = %q, want it to mark the skill as not model-invocable", got)
	}
}

func TestSkillListMultipleSkillsOnePerLine(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "a", Description: "d1", Scope: skill.ScopeUser},
		{Name: "b", Description: "d2", Scope: skill.ScopeProject},
	})
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("SkillList() = %q, want 3 lines (header + 2 skills)", got)
	}
}
