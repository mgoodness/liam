package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/render"
	"github.com/mgoodness/liam/internal/skill"
	"github.com/mgoodness/liam/internal/theme"
)

// maxSlashMatches caps how many candidates the "/"-command popup shows at
// once, matching maxMentionMatches for visual consistency between the two
// popups.
const maxSlashMatches = 8

// builtinCommand is one of liam's four reserved slash commands, carried in
// README order — the fixed order matchSlashQuery lists them in for an
// empty query. This list must stay in sync by hand with submit()'s
// "/quit"/"/clear"/"/compact"/"/skills" switch in tui.go: that switch is
// what actually dispatches each command, this list is only what the popup
// offers, and nothing ties the two together mechanically.
type builtinCommand struct {
	name        string
	description string
}

var builtinCommands = []builtinCommand{
	{"quit", "exit liam"},
	{"clear", "reset the session"},
	{"compact", "condense the conversation history on demand"},
	{"skills", "list discovered skills"},
}

// isBuiltinName reports whether name is one of the four reserved commands.
func isBuiltinName(name string) bool {
	for _, b := range builtinCommands {
		if b.name == name {
			return true
		}
	}
	return false
}

// slashCandidate is one entry the "/"-command popup can offer: a name plus
// a one-line description, sourced from either a builtinCommand or a
// skill.Skill.
type slashCandidate struct {
	name        string
	description string
}

// slashMatch pairs a slashCandidate with the rune indexes into its name
// that a fuzzy query matched — nil for the empty-query fixed listing,
// which never runs through the fuzzy matcher and so has nothing to
// highlight.
type slashMatch struct {
	slashCandidate
	matchedIndexes []int
}

// slashCandidates merges the reserved built-ins with every discovered
// skill into one list the popup can fuzzy-match against. A skill sharing a
// name with a built-in is shadowed — submit()'s documented edge case — and
// is silently dropped here too, rather than showing an entry the popup
// can't actually distinguish from the reserved command.
func slashCandidates(skills []skill.Skill) []slashCandidate {
	out := make([]slashCandidate, 0, len(builtinCommands)+len(skills))
	for _, b := range builtinCommands {
		out = append(out, slashCandidate{name: b.name, description: b.description})
	}
	for _, s := range skills {
		if isBuiltinName(s.Name) {
			continue
		}
		out = append(out, slashCandidate{name: s.Name, description: s.Description})
	}
	return out
}

// matchSlashQuery filters candidates against query. An empty query bypasses
// the fuzzy matcher entirely — its ranking isn't meaningful for an empty
// pattern — and instead lists everything in a fixed order: built-ins first
// (README order), then skills alphabetically. A non-empty query
// fuzzy-matches candidate names only (never descriptions) via
// sahilm/fuzzy, best match first. Either way the result is capped at
// maxSlashMatches.
func matchSlashQuery(candidates []slashCandidate, query string) []slashMatch {
	if query != "" {
		names := make([]string, len(candidates))
		for i, c := range candidates {
			names[i] = c.name
		}
		found := fuzzyRank(query, names, maxSlashMatches)
		out := make([]slashMatch, len(found))
		for i, f := range found {
			out[i] = slashMatch{slashCandidate: candidates[f.Index], matchedIndexes: f.MatchedIndexes}
		}
		return out
	}

	var builtins, skills []slashCandidate
	for _, c := range candidates {
		if isBuiltinName(c.name) {
			builtins = append(builtins, c)
		} else {
			skills = append(skills, c)
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].name < skills[j].name })

	merged := append(builtins, skills...)
	if len(merged) > maxSlashMatches {
		merged = merged[:maxSlashMatches]
	}
	out := make([]slashMatch, len(merged))
	for i, c := range merged {
		out[i] = slashMatch{slashCandidate: c}
	}
	return out
}

// slashState tracks an in-progress "/"-command autocomplete (issue #137):
// active only while the cursor sits inside an unbroken "/query" token that
// starts at row 0, column 0 of the buffer — the same scope
// skillCommandName uses for real activation, just recomputed live on every
// keystroke. Purely cosmetic: it never changes submit()'s dispatch.
type slashState struct {
	active   bool
	query    string
	matches  []slashMatch
	selected int
}

// findSlashStart is findTokenStart with the "/" token's boundary rule: only
// column 0 counts — unlike findMentionStart's "start of line OR after
// whitespace" for "@", a "/" anywhere else ("and/or", a path, a date) is
// ordinary text.
func findSlashStart(line []rune, col int) (int, bool) {
	return findTokenStart(line, col, '/', false)
}

// updateSlash recomputes m.slash from the textarea's current cursor
// position and candidate list, closing the popup the moment the cursor is
// no longer on row 0 or no longer inside an unbroken "/query" token
// starting at column 0. Preserves the selected index across a recompute
// that still has enough matches for it, so typing more of the query
// doesn't reset the user's highlighted match unless it's no longer in
// range.
func (m *Model) updateSlash() {
	if m.input.Line() != 0 {
		m.slash = slashState{}
		return
	}

	lineRunes := []rune(strings.SplitN(m.input.Value(), "\n", 2)[0])
	col := clampColumn(lineRunes, m.input.Column())

	start, ok := findSlashStart(lineRunes, col)
	if !ok {
		m.slash = slashState{}
		return
	}

	query := string(lineRunes[start+1 : col])
	matches := matchSlashQuery(slashCandidates(m.skills), query)

	selected := popupSelectedIndex(m.slash.active, m.slash.selected, len(matches))

	m.slash = slashState{
		active:   true,
		query:    query,
		matches:  matches,
		selected: selected,
	}
}

// selectSlash replaces the active "/query" token with the highlighted
// candidate's full "/name " — trailing space included, cursor readied for
// arguments — then closes the popup. Unlike selectMention, this never
// starts a turn: Tab/Enter here only accepts the suggestion, matching
// issue #137's autocomplete-not-submit decision. A no-op (popup just
// closes) when there are no matches to pick from.
func (m Model) selectSlash() Model {
	if len(m.slash.matches) == 0 {
		m.slash = slashState{}
		return m
	}
	name := m.slash.matches[m.slash.selected].name

	lines := strings.Split(m.input.Value(), "\n")
	lineRunes := []rune(lines[0])
	col := clampColumn(lineRunes, m.input.Column())
	after := string(lineRunes[col:])

	inserted := "/" + name + " "
	lines[0] = inserted + after
	newValue := strings.Join(lines, "\n")

	m.input.SetValue(newValue)
	moveCursorToOffset(&m.input, newValue, len([]rune(inserted)))
	m.slash = slashState{}
	return m
}

// slashRowPrefixWidth is the width, in runes, of the "› /" / "  /" every
// row opens with — both are 3 runes, so it's one constant rather than a
// per-row measurement.
const slashRowPrefixWidth = 3

// slashColumnSeparator sits between the padded name column and the
// description column.
const slashColumnSeparator = " — "

// renderSlashPopup renders the "/"-command autocomplete match list shown
// in the floating popup dialog above the input while ss is active, as a
// two-column table (issue #148): every row's name is padded to a shared
// width so descriptions all start at the same column, and a description
// too long for the popup's width is hard-truncated with a trailing
// ellipsis rather than wrapping. The selected row gets renderMentionPopup's
// whole-line highlight; an unselected row's matched characters are bolded
// per sahilm/fuzzy's MatchedIndexes — issue #137 asked for this to be
// prototyped and judged on how it looks; it earned its keep. width is the
// popup dialog's total on-screen width (renderPopupDialog's Width, border
// included); width <= 0 disables truncation and padding-cap sizing, same
// convention as render.SkillList.
func renderSlashPopup(p theme.Palette, ss slashState, width int) string {
	if len(ss.matches) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Italic(true).Render("  (no matching commands)")
	}

	selected := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Base)).Background(lipgloss.Color(p.Blue)).Bold(true)
	plain := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext))
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Blue)).Bold(true)

	names := make([]string, len(ss.matches))
	for i, mtch := range ss.matches {
		names[i] = mtch.name
	}
	overhead := popupBorderWidth + slashRowPrefixWidth + len([]rune(slashColumnSeparator))
	var nameWidth, descWidth int
	if width > 0 {
		nameWidth, descWidth = render.TableColumns(names, width, overhead, render.MinDescWidth)
	} else {
		nameWidth = render.ColumnWidth(names, 0)
	}

	var b strings.Builder
	for i, mtch := range ss.matches {
		if i > 0 {
			b.WriteByte('\n')
		}
		name, desc := render.FitLabelDesc(mtch.name, mtch.description, nameWidth, descWidth, width)
		padded := fmt.Sprintf("%-*s", nameWidth, name)

		if i == ss.selected {
			b.WriteString(selected.Render(fmt.Sprintf("› /%s%s", padded, descriptionSuffix(desc))))
			continue
		}
		b.WriteString(plain.Render("  /"))
		b.WriteString(renderMatchedName(name, mtch.matchedIndexes, plain, highlight))
		if pad := nameWidth - len([]rune(name)); pad > 0 {
			b.WriteString(plain.Render(strings.Repeat(" ", pad)))
		}
		b.WriteString(plain.Render(descriptionSuffix(desc)))
	}
	return b.String()
}

// descriptionSuffix formats description as " — <description>" for
// appending after a candidate's name, or "" when description is empty so
// the row doesn't render a dangling " — " with nothing after it — no
// built-in currently has an empty description, but a future one (or a
// skill with an empty one) still renders cleanly.
func descriptionSuffix(description string) string {
	if description == "" {
		return ""
	}
	return slashColumnSeparator + description
}

// renderMatchedName renders name rune-by-rune, styling the positions
// matchedIndexes reports (from sahilm/fuzzy's Match.MatchedIndexes) with
// highlight instead of plain. Falls back to plain.Render(name) whole when
// matchedIndexes is empty (the fixed empty-query listing never has any).
func renderMatchedName(name string, matchedIndexes []int, plain, highlight lipgloss.Style) string {
	if len(matchedIndexes) == 0 {
		return plain.Render(name)
	}
	matched := make(map[int]bool, len(matchedIndexes))
	for _, idx := range matchedIndexes {
		matched[idx] = true
	}
	var b strings.Builder
	for i, r := range []rune(name) {
		if matched[i] {
			b.WriteString(highlight.Render(string(r)))
		} else {
			b.WriteString(plain.Render(string(r)))
		}
	}
	return b.String()
}
