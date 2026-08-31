// Package skill implements Agent Skills (agentskills.io) discovery,
// parsing, trust-gating, and activation: SKILL.md files under project and
// user scope directories are discovered, security-scanned, and parsed
// into a catalog the model can activate on demand via a dedicated tool
// (internal/tool.ActivateSkill), following the spec's progressive
// disclosure model — see docs/research/agentskills-spec.md.
package skill

// Scope identifies where a Skill was discovered from.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopeExtra   Scope = "extra"
)

// Skill is one discovered agentskills.io skill: SKILL.md's frontmatter
// fields plus its Markdown body (frontmatter stripped).
type Skill struct {
	Name        string
	Description string
	// DisableModelInvocation is the non-spec disable-model-invocation
	// frontmatter boolean (Claude Code parity): true excludes the skill
	// from ModelCatalog while leaving it reachable via Find for direct
	// (force) activation.
	DisableModelInvocation bool
	Scope                  Scope
	// Dir is the skill's own directory (containing SKILL.md and any
	// bundled scripts/references/assets).
	Dir string
	// Path is SKILL.md's own path.
	Path string
	// Body is SKILL.md's Markdown body, frontmatter stripped — what
	// activation injects into the conversation.
	Body string
}

// DiagnosticLevel classifies a Diagnostic's severity.
type DiagnosticLevel string

const (
	// DiagnosticWarn is a lenient-parsing note about a loaded skill (e.g.
	// a frontmatter name/directory mismatch) — the skill still loads.
	DiagnosticWarn DiagnosticLevel = "warn"
	// DiagnosticSkip means the candidate skill directory was excluded
	// entirely: unparsable frontmatter, a missing/empty description, or a
	// security-scan finding.
	DiagnosticSkip DiagnosticLevel = "skip"
)

// Diagnostic reports something noteworthy Discover ran into for one
// candidate skill directory.
type Diagnostic struct {
	Path    string
	Level   DiagnosticLevel
	Message string
}

// ModelCatalog filters skills down to the ones eligible for the
// model-driven activate_skill catalog — DisableModelInvocation excludes a
// skill from here while leaving it force-activatable via Find.
func ModelCatalog(skills []Skill) []Skill {
	out := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if !s.DisableModelInvocation {
			out = append(out, s)
		}
	}
	return out
}

// Find looks up a skill by name across the full discovered set (including
// ones excluded from ModelCatalog), for direct/force activation — used by
// headless mode's -skill flag, and by the TUI's /skill slash command once
// it exists.
func Find(skills []Skill, name string) (Skill, bool) {
	for _, s := range skills {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}
