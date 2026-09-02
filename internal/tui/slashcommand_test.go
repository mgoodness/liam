package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

func TestQuitHasADescription(t *testing.T) {
	// issue #148: /quit previously had no description, unlike the other
	// three built-ins. The popup (and README, kept in sync by hand) now
	// give it one in the same tone as the rest.
	got := slashCandidates(nil)
	for _, c := range got {
		if c.name == "quit" && c.description == "" {
			t.Error("quit's description is empty, want a real description matching the other built-ins")
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

// TestRenderSlashPopupAlignsDescriptionColumn covers the AC that every
// visible row's description starts at the same column regardless of name
// length (issue #148). Stripped of ANSI styling, since a selected row and
// an unselected/highlighted row build their text through a different
// number of lipgloss.Render calls that would otherwise throw off a raw
// byte-offset comparison despite being visually aligned.
func TestRenderSlashPopupAlignsDescriptionColumn(t *testing.T) {
	p := New(agent.Loop{}, config.Config{}, nil).pal
	ss := slashState{
		active: true,
		matches: []slashMatch{
			{slashCandidate: slashCandidate{name: "a", description: "short"}},
			{slashCandidate: slashCandidate{name: "much-longer-name", description: "also has a description"}},
		},
	}

	got := ansi.Strip(renderSlashPopup(p, ss, 80))
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("renderSlashPopup() = %d lines, want 2", len(lines))
	}
	// Rune index, not byte index: "›" (the selected row's leading marker)
	// is a multi-byte rune, so a byte offset would report misalignment
	// that isn't actually there on screen.
	d0 := runeIndex(lines[0], "—")
	d1 := runeIndex(lines[1], "—")
	if d0 != d1 {
		t.Errorf("description column starts at %d and %d, want the same column\n%q\n%q", d0, d1, lines[0], lines[1])
	}
}

// runeIndex returns the rune (not byte) index of sep's first occurrence in
// s, or -1 if absent.
func runeIndex(s, sep string) int {
	i := strings.Index(s, sep)
	if i < 0 {
		return -1
	}
	return len([]rune(s[:i]))
}

// TestRenderSlashPopupTruncatesLongDescription covers the AC that a
// description too long for the popup's width is hard-truncated with a
// trailing ellipsis rather than wrapping onto a second line.
func TestRenderSlashPopupTruncatesLongDescription(t *testing.T) {
	p := New(agent.Loop{}, config.Config{}, nil).pal
	ss := slashState{
		active: true,
		matches: []slashMatch{
			{slashCandidate: slashCandidate{name: "a", description: strings.Repeat("x", 200)}},
		},
	}

	width := 40
	got := ansi.Strip(renderSlashPopup(p, ss, width))
	if strings.Contains(got, "\n") {
		t.Errorf("renderSlashPopup() wrapped onto a second line, want one truncated row: %q", got)
	}
	if visible := len([]rune(got)); visible > width-popupBorderWidth {
		t.Errorf("row width = %d runes, want <= %d (content width)", visible, width-popupBorderWidth)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("row = %q, want a hard-truncated ellipsis", got)
	}
}

// TestRenderSlashPopupPreservesHighlightingForEqualLengthNames is a
// regression guard for the AC that fuzzy-match highlighting is unchanged
// for content the table layout doesn't need to touch: when every visible
// name is already the same length, no padding is added, so a row's output
// should be byte-identical to building it the pre-#148 way.
func TestRenderSlashPopupPreservesHighlightingForEqualLengthNames(t *testing.T) {
	p := New(agent.Loop{}, config.Config{}, nil).pal
	ss := slashState{
		active:   true,
		selected: 1,
		matches: []slashMatch{
			{slashCandidate: slashCandidate{name: "abcd", description: "d1"}, matchedIndexes: []int{0, 2}},
			{slashCandidate: slashCandidate{name: "wxyz", description: "d2"}},
		},
	}

	got := renderSlashPopup(p, ss, 0)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("renderSlashPopup() = %d lines, want 2", len(lines))
	}

	plain := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext))
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Blue)).Bold(true)
	want := plain.Render("  /") + renderMatchedName("abcd", []int{0, 2}, plain, highlight) + plain.Render(descriptionSuffix("d1"))
	if lines[0] != want {
		t.Errorf("renderSlashPopup() row 0 = %q, want %q", lines[0], want)
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
