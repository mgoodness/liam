package render

import (
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/skill"
)

func TestSkillListEmpty(t *testing.T) {
	if got := SkillList(nil, 80); got != "No skills discovered." {
		t.Errorf("SkillList(nil, 80) = %q, want %q", got, "No skills discovered.")
	}
}

func TestSkillListFormatsNameScopeDescription(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "commit-messages", Description: "Write conventional commit messages.", Scope: skill.ScopeUser},
	}, 80)
	want := "1 skill(s) discovered:\n  commit-messages (user) — Write conventional commit messages."
	if got != want {
		t.Errorf("SkillList() = %q, want %q", got, want)
	}
}

func TestSkillListMarksDisableModelInvocation(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "hidden", Description: "d", Scope: skill.ScopeProject, DisableModelInvocation: true},
	}, 80)
	if !strings.Contains(got, "[not model-invocable]") {
		t.Errorf("SkillList() = %q, want it to mark the skill as not model-invocable", got)
	}
}

func TestSkillListMultipleSkillsOnePerLine(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "a", Description: "d1", Scope: skill.ScopeUser},
		{Name: "b", Description: "d2", Scope: skill.ScopeProject},
	}, 80)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("SkillList() = %q, want 3 lines (header + 2 skills)", got)
	}
}

// TestSkillListAlignsDescriptionColumn covers the AC that every row's
// description starts at the same column regardless of how long its
// "name (scope)" label is (issue #148).
func TestSkillListAlignsDescriptionColumn(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "a", Description: "short name", Scope: skill.ScopeUser},
		{Name: "much-longer-skill-name", Description: "long name", Scope: skill.ScopeProject},
	}, 80)

	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("SkillList() = %q, want 3 lines", got)
	}
	dashA := runeIndex(lines[1], "—")
	dashB := runeIndex(lines[2], "—")
	if dashA != dashB {
		t.Errorf("description column starts at %d and %d, want the same column\n%s", dashA, dashB, got)
	}
}

// TestSkillListTruncatesLongDescriptionToWidth covers the AC that a
// description too long for width is hard-truncated with a trailing
// ellipsis rather than left to wrap.
func TestSkillListTruncatesLongDescriptionToWidth(t *testing.T) {
	got := SkillList([]skill.Skill{
		{Name: "a", Description: strings.Repeat("x", 200), Scope: skill.ScopeUser},
	}, 40)

	lines := strings.Split(got, "\n")
	line := lines[1]
	if got := len([]rune(line)); got > 40 {
		t.Errorf("row width = %d runes, want <= 40 (width)", got)
	}
	if !strings.HasSuffix(line, "…") {
		t.Errorf("line = %q, want a hard-truncated ellipsis", line)
	}
}

// TestSkillListTruncatesOverflowingLabel is a regression guard: capping
// labelWidth alone isn't enough if an individual label longer than the cap
// is never actually cut down to it — the row would still overflow width
// and its "—" would land at a different column than shorter rows'.
func TestSkillListTruncatesOverflowingLabel(t *testing.T) {
	width := 40
	got := SkillList([]skill.Skill{
		{Name: "a", Description: "d", Scope: skill.ScopeUser},
		{Name: strings.Repeat("x", 100), Description: "short", Scope: skill.ScopeProject},
	}, width)

	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("SkillList() = %q, want 3 lines", got)
	}
	for i, line := range lines[1:] {
		if n := len([]rune(line)); n > width {
			t.Errorf("line %d width = %d runes, want <= %d (width): %q", i+1, n, width, line)
		}
	}
	dashA := runeIndex(lines[1], "—")
	dashB := runeIndex(lines[2], "—")
	if dashA != dashB {
		t.Errorf("description column starts at %d and %d, want the same column\n%s", dashA, dashB, got)
	}
}

// TestSkillListNoWidthSkipsTruncation covers width <= 0 (a headless caller
// with no terminal, or a test that never sends a window-size message)
// disabling truncation entirely, matching statusline.truncateRows'
// convention.
func TestSkillListNoWidthSkipsTruncation(t *testing.T) {
	desc := strings.Repeat("x", 200)
	got := SkillList([]skill.Skill{{Name: "a", Description: desc, Scope: skill.ScopeUser}}, 0)

	if !strings.Contains(got, desc) {
		t.Errorf("SkillList(..., 0) truncated the description, want it left untouched")
	}
}

// runeIndex returns the rune (not byte) index of sep's first occurrence in
// s, or -1 if absent — a truncated label can contain the multi-byte "…"
// ellipsis, which throws off a raw strings.Index byte offset despite the
// rune-width column alignment being correct.
func runeIndex(s, sep string) int {
	i := strings.Index(s, sep)
	if i < 0 {
		return -1
	}
	return len([]rune(s[:i]))
}
