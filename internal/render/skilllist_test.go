package render

import (
	"reflect"
	"testing"

	"github.com/mgoodness/liam/internal/skill"
)

func TestSkillGroupsEmpty(t *testing.T) {
	if got := SkillGroups(nil); len(got) != 0 {
		t.Errorf("SkillGroups(nil) = %+v, want no groups", got)
	}
}

func TestSkillGroupsOrderedMostSpecificFirst(t *testing.T) {
	skills := []skill.Skill{
		{Name: "extra-skill", Scope: skill.ScopeExtra},
		{Name: "user-skill", Scope: skill.ScopeUser},
		{Name: "project-skill", Scope: skill.ScopeProject},
	}
	got := SkillGroups(skills)
	if len(got) != 3 {
		t.Fatalf("SkillGroups() = %d groups, want 3", len(got))
	}
	wantOrder := []skill.Scope{skill.ScopeProject, skill.ScopeUser, skill.ScopeExtra}
	for i, scope := range wantOrder {
		if got[i].Scope != scope {
			t.Errorf("group %d scope = %q, want %q", i, got[i].Scope, scope)
		}
	}
}

func TestSkillGroupsOmitsEmptyScopes(t *testing.T) {
	skills := []skill.Skill{{Name: "only-one", Scope: skill.ScopeUser}}
	got := SkillGroups(skills)
	if len(got) != 1 || got[0].Scope != skill.ScopeUser {
		t.Errorf("SkillGroups() = %+v, want a single user-scope group", got)
	}
}

func TestSkillGroupsSortsWithinGroupByName(t *testing.T) {
	skills := []skill.Skill{
		{Name: "zebra", Scope: skill.ScopeUser},
		{Name: "apple", Scope: skill.ScopeUser},
	}
	got := SkillGroups(skills)
	want := []string{"apple", "zebra"}
	names := make([]string, len(got[0].Skills))
	for i, s := range got[0].Skills {
		names[i] = s.Name
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("group skill order = %v, want %v", names, want)
	}
}
