package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/mgoodness/liam/internal/skill"
)

// ActivateSkill is the dedicated tool through which the model activates a
// discovered skill by name — agentskills.io's "Tier 2" progressive
// disclosure step. The catalog (name + description per skill) is embedded
// in this tool's own Description rather than the system prompt, and
// Parameters name-enum-constrains the "name" argument against
// hallucinated skill names.
type ActivateSkill struct {
	// Catalog is the set of skills the model may activate. Callers pass
	// skill.ModelCatalog's output — skills with DisableModelInvocation
	// set are already excluded.
	Catalog []skill.Skill
}

func (a ActivateSkill) Name() string { return "activate_skill" }

func (a ActivateSkill) Description() string {
	var b strings.Builder
	b.WriteString("Activate a skill by name, injecting its full instructions into the conversation. ")
	b.WriteString("Use this when a skill's description matches the current task. Available skills:\n")
	for _, s := range a.Catalog {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return b.String()
}

func (a ActivateSkill) Parameters() Schema {
	names := make([]string, len(a.Catalog))
	for i, s := range a.Catalog {
		names[i] = s.Name
	}
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the skill to activate.",
				"enum":        names,
			},
		},
		"required": []string{"name"},
	}
}

func (a ActivateSkill) Safety() Safety {
	return Safety{SideEffect: SideEffectRead}
}

func (a ActivateSkill) Run(_ context.Context, args map[string]any) Result {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return errorResult(`activate_skill: "name" argument is required`)
	}

	if s, found := skill.Find(a.Catalog, name); found {
		return Result{Content: s.Body}
	}
	return errorResult(fmt.Sprintf("activate_skill: unknown skill %q", name))
}
