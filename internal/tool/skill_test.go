package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/skill"
)

func testCatalog() []skill.Skill {
	return []skill.Skill{
		{Name: "commit-messages", Description: "Write conventional commit messages.", Body: "# commit-messages\n\nUse Conventional Commits."},
		{Name: "code-review", Description: "Review a diff for standards and spec compliance.", Body: "# code-review\n\nCheck standards and spec."},
	}
}

func TestActivateSkillSafety(t *testing.T) {
	want := Safety{SideEffect: SideEffectRead}
	if got := (ActivateSkill{}).Safety(); got != want {
		t.Errorf("Safety() = %+v, want %+v", got, want)
	}
}

func TestActivateSkillDescriptionEmbedsCatalog(t *testing.T) {
	desc := (ActivateSkill{Catalog: testCatalog()}).Description()
	for _, want := range []string{"commit-messages", "Write conventional commit messages.", "code-review", "Review a diff"} {
		if !strings.Contains(desc, want) {
			t.Errorf("Description() = %q, want it to contain %q", desc, want)
		}
	}
}

func TestActivateSkillParametersEnumConstrainsNames(t *testing.T) {
	params := (ActivateSkill{Catalog: testCatalog()}).Parameters()
	props := params["properties"].(map[string]any)
	nameSchema := props["name"].(map[string]any)
	enum, ok := nameSchema["enum"].([]string)
	if !ok {
		t.Fatalf("enum = %#v, want []string", nameSchema["enum"])
	}
	if len(enum) != 2 || enum[0] != "commit-messages" || enum[1] != "code-review" {
		t.Errorf("enum = %v, want [commit-messages code-review]", enum)
	}
}

func TestActivateSkillRunInjectsBody(t *testing.T) {
	got := (ActivateSkill{Catalog: testCatalog()}).Run(context.Background(), map[string]any{"name": "commit-messages"})
	if got.IsError {
		t.Fatalf("Run() IsError = true, Content = %q", got.Content)
	}
	if got.Content != "# commit-messages\n\nUse Conventional Commits." {
		t.Errorf("Content = %q, want the skill's body", got.Content)
	}
}

func TestActivateSkillRunUnknownName(t *testing.T) {
	got := (ActivateSkill{Catalog: testCatalog()}).Run(context.Background(), map[string]any{"name": "nonexistent"})
	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}

func TestActivateSkillRunMissingNameArg(t *testing.T) {
	got := (ActivateSkill{Catalog: testCatalog()}).Run(context.Background(), map[string]any{})
	if !got.IsError {
		t.Fatalf("Run() IsError = false, want true")
	}
}
