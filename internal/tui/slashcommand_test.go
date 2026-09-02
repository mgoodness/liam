package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/skill"
)

func TestFindSlashStartAtColumnZero(t *testing.T) {
	line := []rune("/clear")
	start, ok := findSlashStart(line, len(line))
	if !ok || start != 0 {
		t.Errorf("findSlashStart = %d, %v, want 0, true", start, ok)
	}
}

func TestFindSlashStartRejectsMidLineSlash(t *testing.T) {
	// Unlike "@", a "/" preceded by non-whitespace (or anything but the
	// very start of the line) is never a command — e.g. "and/or".
	line := []rune("and/or")
	if _, ok := findSlashStart(line, len(line)); ok {
		t.Error("findSlashStart found a token mid-line, want none")
	}
}

func TestFindSlashStartRejectsAfterWhitespace(t *testing.T) {
	// findMentionStart accepts "@" after whitespace; findSlashStart must
	// not extend the same leniency to "/" — only column 0 counts.
	line := []rune("run /clear now")
	if _, ok := findSlashStart(line, len("run /clear")); ok {
		t.Error("findSlashStart found a token after whitespace, want none (column 0 only)")
	}
}

func TestFindSlashStartClosesOnWhitespace(t *testing.T) {
	line := []rune("/clear now")
	if _, ok := findSlashStart(line, len(line)); ok {
		t.Error("findSlashStart found a token past whitespace, want none")
	}
}

func TestSlashCandidatesIncludesBuiltinsAndSkills(t *testing.T) {
	skills := []skill.Skill{{Name: "implement", Description: "Implement a feature."}}
	got := slashCandidates(skills)

	if len(got) != len(builtinCommands)+1 {
		t.Fatalf("slashCandidates returned %d entries, want %d", len(got), len(builtinCommands)+1)
	}
}

func TestSlashCandidatesHidesSkillShadowedByBuiltin(t *testing.T) {
	skills := []skill.Skill{{Name: "clear", Description: "A skill that happens to be named like a built-in."}}
	got := slashCandidates(skills)

	if len(got) != len(builtinCommands) {
		t.Fatalf("slashCandidates returned %d entries, want exactly the %d built-ins (shadowed skill hidden)", len(got), len(builtinCommands))
	}
	for _, c := range got {
		if c.name == "clear" && c.description != "reset the session" {
			t.Errorf("candidate %q description = %q, want the built-in's description, not the shadowed skill's", c.name, c.description)
		}
	}
}

func TestQuitHasNoDescriptionMatchingReadmesBareListing(t *testing.T) {
	// The README lists "/quit" with no parenthetical description, unlike
	// the other three built-ins — the popup must match that rather than
	// inventing copy.
	got := slashCandidates(nil)
	for _, c := range got {
		if c.name == "quit" && c.description != "" {
			t.Errorf("quit's description = %q, want empty (README lists it bare)", c.description)
		}
	}
}

func TestDescriptionSuffixEmptyOmitsDash(t *testing.T) {
	if got := descriptionSuffix(""); got != "" {
		t.Errorf("descriptionSuffix(\"\") = %q, want \"\" (no dangling \" — \")", got)
	}
	if got := descriptionSuffix("reset the session"); got != " — reset the session" {
		t.Errorf("descriptionSuffix(\"reset the session\") = %q, want \" — reset the session\"", got)
	}
}

func TestMatchSlashQueryEmptyListsBuiltinsThenSkillsAlphabetically(t *testing.T) {
	skills := []skill.Skill{
		{Name: "zeta", Description: "z"},
		{Name: "alpha", Description: "a"},
	}
	got := matchSlashQuery(slashCandidates(skills), "")

	wantOrder := []string{"quit", "clear", "compact", "skills", "alpha", "zeta"}
	if len(got) != len(wantOrder) {
		t.Fatalf("matchSlashQuery(\"\") returned %d entries, want %d", len(got), len(wantOrder))
	}
	for i, name := range wantOrder {
		if got[i].name != name {
			t.Errorf("matchSlashQuery(\"\")[%d] = %q, want %q", i, got[i].name, name)
		}
	}
}

func TestMatchSlashQueryFuzzyMatchesNameOnly(t *testing.T) {
	skills := []skill.Skill{{Name: "implement", Description: "mentions clear reasoning"}}
	got := matchSlashQuery(slashCandidates(skills), "clear")

	if len(got) != 1 || got[0].name != "clear" {
		t.Fatalf("matchSlashQuery(\"clear\") = %v, want only the \"clear\" built-in (description text must not match)", got)
	}
}

func TestMatchSlashQueryCapsAtMaxSlashMatches(t *testing.T) {
	var skills []skill.Skill
	for i := 0; i < maxSlashMatches+5; i++ {
		skills = append(skills, skill.Skill{Name: "skill" + string(rune('a'+i)), Description: "x"})
	}
	got := matchSlashQuery(slashCandidates(skills), "skill")
	if len(got) != maxSlashMatches {
		t.Errorf("matchSlashQuery returned %d matches, want capped at %d", len(got), maxSlashMatches)
	}
}

func TestTypingSlashAtBufferStartOpensPopup(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)

	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	mm := next.(Model)

	if !mm.slash.active {
		t.Fatal("slash.active = false after typing \"/\" at buffer start, want true")
	}
	if len(mm.slash.matches) != len(builtinCommands) {
		t.Errorf("slash.matches = %d entries, want %d (all built-ins, empty query)", len(mm.slash.matches), len(builtinCommands))
	}
}

func TestSlashPopupClosesAfterFirstNewline(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)

	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	mm := next.(Model)

	if mm.slash.active {
		t.Error("slash.active = true for a \"/\" typed on the second line, want false (row 0 only)")
	}
}

func TestSlashClosesOnMidLineSlash(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	for _, r := range "and/or" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}

	if m.slash.active {
		t.Error("slash.active = true for a \"/\" mid-line, want false")
	}
}

func TestTabAutocompletesSlashCommandWithTrailingSpace(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	for _, r := range "/cle" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	mm := next.(Model)

	if mm.slash.active {
		t.Error("slash.active = true after Tab-accepting a suggestion, want false")
	}
	if mm.input.Value() != "/clear " {
		t.Errorf("input.Value() = %q, want %q", mm.input.Value(), "/clear ")
	}
	if cmd != nil {
		t.Error("Tab-accepting a suggestion returned a non-nil cmd, want nil (no turn started)")
	}
}

func TestEnterStillSubmitsWhileSlashPopupIsActive(t *testing.T) {
	// Regression guard: unlike "@"-mention (where Enter always accepts,
	// since there's no pre-existing "submit a bare mention" expectation),
	// a fully-typed unambiguous command like "/quit" followed by Enter
	// must submit immediately, matching cmd/liam's own
	// TestRunNoArgsOpensInteractiveTUI, which types "/quit\r" as one
	// uninterrupted script. Only Tab autocompletes; Enter never gets
	// intercepted by the popup.
	m := New(agent.Loop{}, config.Config{}, nil)
	for _, r := range "/quit" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}
	if !m.slash.active {
		t.Fatal("slash.active = false after typing \"/quit\", want true (setup broken)")
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm := next.(Model)

	if cmd == nil {
		t.Fatal("Enter on \"/quit\" with the popup active returned a nil cmd, want tea.Quit (submit() must run)")
	}
	if mm.slash.active {
		t.Error("slash.active = true after Enter submitted, want false (popup must not linger over an emptied input)")
	}
}

func TestArrowKeysMoveSlashSelectionNotHistory(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.hist.add("earlier message")

	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mm := next.(Model)

	if mm.slash.selected != 1 {
		t.Errorf("slash.selected = %d, want 1 (moved within the popup)", mm.slash.selected)
	}
	if mm.input.Value() != "/" {
		t.Errorf("input.Value() = %q, want \"/\" (history must not have been recalled)", mm.input.Value())
	}
}

func TestEscClosesSlashPopupWithoutChangingText(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	mm := next.(Model)

	if mm.slash.active {
		t.Error("slash.active = true after Esc, want false")
	}
	if mm.input.Value() != "/" {
		t.Errorf("input.Value() = %q, want \"/\" unchanged by Esc", mm.input.Value())
	}
}
