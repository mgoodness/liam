package tui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/theme"
)

// maxMentionMatches caps how many find results the "@" popup shows at once
// — a small on-screen list, unrelated to tool.MaxSearchResults' much larger
// model-facing cap.
const maxMentionMatches = 8

// mentionState tracks an in-progress "@"-file-reference autocomplete
// (issue #58): active while the cursor sits inside an unbroken "@query"
// token, closing automatically the moment that's no longer true.
type mentionState struct {
	active   bool
	line     int // row (textarea.Line()) the mention was opened on
	start    int // rune column of the '@' on that row
	query    string
	matches  []mentionMatch
	selected int
}

// mentionMatch pairs a workspace-relative path with the rune indexes into
// it that a fuzzy query matched — nil for the empty-query listing, which
// never runs through the fuzzy matcher and so has nothing to highlight.
type mentionMatch struct {
	path           string
	matchedIndexes []int
}

// fileRefRangeRe splits a mention query's optional trailing ":42" or
// ":10-20" line-range suffix from the file part it filters on.
var fileRefRangeRe = regexp.MustCompile(`^(.*):(\d+)(?:-(\d+))?$`)

// fileReference is a mention query split into the file-search part and an
// optional 1-indexed inclusive line range parsed from a trailing
// ":42"/":10-20" suffix.
type fileReference struct {
	query    string // the file-search part, with any range suffix stripped
	start    int
	end      int
	hasRange bool
}

// parseFileReference splits query per fileReference's doc comment. hasRange
// is false (and start/end are 0, query returned unchanged) when query has no
// ":N"/":N-M" suffix, or the suffix doesn't parse into a sane non-empty
// ascending range.
func parseFileReference(query string) fileReference {
	m := fileRefRangeRe.FindStringSubmatch(query)
	if m == nil {
		return fileReference{query: query}
	}

	start, err := strconv.Atoi(m[2])
	if err != nil || start < 1 {
		return fileReference{query: query}
	}
	end := start
	if m[3] != "" {
		end, err = strconv.Atoi(m[3])
		if err != nil || end < start {
			return fileReference{query: query}
		}
	}
	return fileReference{query: m[1], start: start, end: end, hasRange: true}
}

// findMentionStart is findTokenStart with the "@" token's boundary rule: a
// trigger preceded by whitespace or the start of the line. findTokenStart,
// clampColumn, popupSelectedIndex, and moveCursorToOffset — the pieces of
// this file's logic with nothing "@"-specific about them — live in
// popup.go, the shared component both this file and slashcommand.go call
// into (issue #139's Consolidation).
func findMentionStart(line []rune, col int) (int, bool) {
	return findTokenStart(line, col, '@', true)
}

// matchMentionQuery ranks paths against query using sahilm/fuzzy — the same
// matcher matchSlashQuery uses for the "/"-command popup, reused here for
// ranking consistency between the two popups (issue #155). An empty query
// bypasses the fuzzy matcher — its ranking isn't meaningful for an empty
// pattern — and lists paths in the order findSearcher.Find returned them.
// Either way the result is capped at maxMentionMatches.
func matchMentionQuery(paths []string, query string) []mentionMatch {
	if query == "" {
		if len(paths) > maxMentionMatches {
			paths = paths[:maxMentionMatches]
		}
		out := make([]mentionMatch, len(paths))
		for i, p := range paths {
			out[i] = mentionMatch{path: p}
		}
		return out
	}

	found := fuzzyRank(query, paths, maxMentionMatches)
	out := make([]mentionMatch, len(found))
	for i, f := range found {
		out[i] = mentionMatch{path: paths[f.Index], matchedIndexes: f.MatchedIndexes}
	}
	return out
}

// mentionCandidatePaths returns the paths matchMentionQuery should rank:
// findSearcher's unfiltered whole-workspace listing, unioned with its own
// substring-filtered results for query (skipped entirely when query is
// empty, since that's the same request). The union matters because
// tool.FindSearcher caps its own result count (deliberately — its
// unindexed, substring-only contract stays untouched here, per issue #155)
// — in a workspace with more files than that cap, the unfiltered listing
// alone only covers whichever files the backend's tree walk happens to
// visit first, so an exact substring match past that boundary would
// otherwise never reach the fuzzy ranker at all (issue #155 code review).
func (m *Model) mentionCandidatePaths(query string) []string {
	all, _, err := m.findSearcher.Find(context.Background(), "")
	if err != nil {
		all = nil
	}
	if query == "" {
		return all
	}

	substring, _, err := m.findSearcher.Find(context.Background(), query)
	if err != nil {
		return all
	}

	seen := make(map[string]bool, len(all))
	out := make([]string, 0, len(all)+len(substring))
	for _, p := range all {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range substring {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// updateMention recomputes m.mention from the textarea's current cursor
// position, closing the popup when the cursor no longer sits inside an
// unbroken "@query" token or no findSearcher was attached (WithFindSearcher
// unset). Preserves the selected index across a recompute that still
// targets the same token, so typing more of the query doesn't reset the
// user's highlighted match unless it's no longer in range.
func (m *Model) updateMention() {
	if m.findSearcher == nil {
		m.mention = mentionState{}
		return
	}

	lines := strings.Split(m.input.Value(), "\n")
	row := m.input.Line()
	if row >= len(lines) {
		m.mention = mentionState{}
		return
	}
	lineRunes := []rune(lines[row])
	col := clampColumn(lineRunes, m.input.Column())

	start, ok := findMentionStart(lineRunes, col)
	if !ok {
		m.mention = mentionState{}
		return
	}

	query := string(lineRunes[start+1 : col])
	ref := parseFileReference(query)

	matches := matchMentionQuery(m.mentionCandidatePaths(ref.query), ref.query)

	// Carry the highlight over only when the popup stays active on the
	// same "@" token (guarded by mention.line/start above) — see
	// popupSelectedIndex.
	selected := popupSelectedIndex(m.mention.active && m.mention.line == row && m.mention.start == start, m.mention.selected, len(matches))

	m.mention = mentionState{
		active:   true,
		line:     row,
		start:    start,
		query:    query,
		matches:  matches,
		selected: selected,
	}
}

// selectMention replaces the active "@query" token with a plain "@path" (or
// "@path:N"/"@path:N-M" when a range was typed) reference to the highlighted
// match, then closes the popup. No file is read — issue #155 moved this
// from inlining file content to a bare reference, since the model can read
// the file itself once it sees the path. A no-op (menu just closes) when
// there are no matches to pick from.
func (m Model) selectMention() Model {
	if len(m.mention.matches) == 0 {
		m.mention = mentionState{}
		return m
	}
	path := m.mention.matches[m.mention.selected].path
	ref := parseFileReference(m.mention.query)

	inserted := "@" + path
	if ref.hasRange {
		if ref.end != ref.start {
			inserted = fmt.Sprintf("@%s:%d-%d", path, ref.start, ref.end)
		} else {
			inserted = fmt.Sprintf("@%s:%d", path, ref.start)
		}
	}

	lines := strings.Split(m.input.Value(), "\n")
	lineRunes := []rune(lines[m.mention.line])
	col := clampColumn(lineRunes, m.input.Column())

	before := string(lineRunes[:m.mention.start])
	after := string(lineRunes[col:])

	var prefixOffset int
	for _, l := range lines[:m.mention.line] {
		prefixOffset += len([]rune(l)) + 1
	}
	offset := prefixOffset + len([]rune(before)) + len([]rune(inserted))

	lines[m.mention.line] = before + inserted + after
	newValue := strings.Join(lines, "\n")

	m.input.SetValue(newValue)
	moveCursorToOffset(&m.input, newValue, offset)
	m.mention = mentionState{}
	return m
}

// renderMentionPopup renders the "@"-autocomplete match list shown in the
// floating popup dialog above the input while ms is active. An unselected
// row's fuzzy-matched characters are bolded per sahilm/fuzzy's
// MatchedIndexes, mirroring renderSlashPopup's highlighting.
func renderMentionPopup(p theme.Palette, ms mentionState) string {
	if len(ms.matches) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Italic(true).Render("  (no matching files)")
	}

	selected := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Base)).Background(lipgloss.Color(p.Blue)).Bold(true)
	plain := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext))
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Blue)).Bold(true)

	var b strings.Builder
	for i, mtch := range ms.matches {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i == ms.selected {
			b.WriteString(selected.Render("› " + mtch.path))
			continue
		}
		b.WriteString(plain.Render("  "))
		b.WriteString(renderMatchedName(mtch.path, mtch.matchedIndexes, plain, highlight))
	}
	return b.String()
}
