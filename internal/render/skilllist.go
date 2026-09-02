package render

import (
	"sort"

	"github.com/mgoodness/liam/internal/skill"
)

// SkillGroup buckets one scope's skills together for /skills' grouped
// display (issue #82).
type SkillGroup struct {
	Scope  skill.Scope
	Skills []skill.Skill
}

// skillGroupOrder is the fixed scope order SkillGroups renders groups in —
// most specific first: a project skill applies to this one repo, a user
// skill to this whole machine, an extra-path skill to wherever the
// skills.paths config happens to point. This is a display convention, not
// a restatement of skill.Discover's own collision precedence (which scans
// user, then extra, then project last — so a same-named skill only
// resolves project-wins-over-the-rest, and says nothing about user vs.
// extra).
var skillGroupOrder = []skill.Scope{skill.ScopeProject, skill.ScopeUser, skill.ScopeExtra}

// SkillGroups buckets skills by scope in skillGroupOrder, each group's
// skills sorted by name (matching skill.Discover's overall sort, just
// scoped down to the group). A scope with no discovered skills contributes
// no group at all, so callers never need to special-case an empty one.
func SkillGroups(skills []skill.Skill) []SkillGroup {
	byScope := make(map[skill.Scope][]skill.Skill, len(skillGroupOrder))
	for _, s := range skills {
		byScope[s.Scope] = append(byScope[s.Scope], s)
	}

	groups := make([]SkillGroup, 0, len(skillGroupOrder))
	for _, scope := range skillGroupOrder {
		ss := byScope[scope]
		if len(ss) == 0 {
			continue
		}
		sort.Slice(ss, func(i, j int) bool { return ss[i].Name < ss[j].Name })
		groups = append(groups, SkillGroup{Scope: scope, Skills: ss})
	}
	return groups
}
