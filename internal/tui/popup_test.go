package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
)

func TestFindTokenStartRequiresAllowedBoundaryBeforeTrigger(t *testing.T) {
	cases := []struct {
		name          string
		line          string
		col           int
		trigger       rune
		spacePreceded bool
		wantStart     int
		wantOK        bool
	}{
		// "@" (spacePreceded=true): a trigger preceded by whitespace or the
		// start of the line opens a token.
		{"mention at line start", "@main.go", len("@main.go"), '@', true, 0, true},
		{"mention after whitespace", "read @main.go", len("read @main.go"), '@', true, 5, true},
		{"mention mid-word rejected", "email@example.com", len("email@example.com"), '@', true, 0, false},
		{"mention broken by whitespace", "@main.go please", len("@main.go please"), '@', true, 0, false},

		// "/" (spacePreceded=false): only column 0 opens a token.
		{"slash at column zero", "/clear", len("/clear"), '/', false, 0, true},
		{"slash mid-line rejected", "and/or", len("and/or"), '/', false, 0, false},
		{"slash after whitespace rejected", "run /clear now", len("run /clear"), '/', false, 0, false},
		{"slash broken by whitespace", "/clear now", len("/clear now"), '/', false, 0, false},

		{"empty line at col zero", "", 0, '@', true, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, ok := findTokenStart([]rune(tc.line), tc.col, tc.trigger, tc.spacePreceded)
			if ok != tc.wantOK || start != tc.wantStart {
				t.Errorf("findTokenStart(%q, %d, %q, %v) = (%d, %v), want (%d, %v)",
					tc.line, tc.col, tc.trigger, tc.spacePreceded, start, ok, tc.wantStart, tc.wantOK)
			}
		})
	}
}

func TestPopupSelectedIndexCarriesSelectionWhileInRange(t *testing.T) {
	cases := []struct {
		name         string
		active       bool
		prevSelected int
		newLen       int
		want         int
	}{
		{"inactive resets to top", false, 3, 8, 0},
		{"kept while still in range", true, 3, 8, 3},
		{"top index kept at zero", true, 0, 8, 0},
		{"reset when at new list end", true, 8, 8, 0},
		{"reset when past new list end", true, 9, 8, 0},
		{"reset when list emptied", true, 0, 0, 0},
		// mention's updateMention guards "same token" before passing
		// active — e.g. the popup's "@" token was at column 0 and the
		// cursor moved to a new "@" at column 5, or the cursor re-entered
		// a different token. Piped as inactive into this helper.
		{"reset when helper inactive but index in range", false, 2, 8, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := popupSelectedIndex(tc.active, tc.prevSelected, tc.newLen); got != tc.want {
				t.Errorf("popupSelectedIndex(%v, %d, %d) = %d, want %d",
					tc.active, tc.prevSelected, tc.newLen, got, tc.want)
			}
		})
	}
}

// TestRenderPopupDialogFixedHeightRegardlessOfContent covers the AC that a
// popup dialog is a constant popupDialogHeight rows tall and width columns
// wide (issue #139) whether its match-list content is one line, several
// lines, or the full maxMentionMatches cap — no per-keystroke resizing as
// results narrow or widen.
func TestRenderPopupDialogFixedHeightRegardlessOfContent(t *testing.T) {
	p := New(agent.Loop{}, config.Config{}, nil).pal
	width := 40

	for _, n := range []int{1, 2, maxMentionMatches} {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = "match"
		}
		content := strings.Join(lines, "\n")

		dialog := renderPopupDialog(p, width, content)
		if got := lipgloss.Height(dialog); got != popupDialogHeight {
			t.Errorf("n=%d: renderPopupDialog height = %d, want %d", n, got, popupDialogHeight)
		}
		if got := lipgloss.Width(dialog); got != width {
			t.Errorf("n=%d: renderPopupDialog width = %d, want %d", n, got, width)
		}
	}
}

// TestSyncViewportDimsReservesPopupDialogHeightWhenActive covers the AC
// that the popup dialog's height is carved out of the viewport's height
// budget only while a popup is actually active — issue #139's research
// found syncViewportDims reserved nothing for either popup before this.
func TestSyncViewportDimsReservesPopupDialogHeightWhenActive(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	before := m.viewport.Height()

	m.mention = mentionState{active: true, matches: []mentionMatch{{path: "a.go"}}}
	m.syncViewportDims()
	if got, want := m.viewport.Height(), before-popupDialogHeight; got != want {
		t.Errorf("mention active: viewport.Height() = %d, want %d (popupDialogHeight reserved)", got, want)
	}

	m.mention = mentionState{}
	m.slash = slashState{active: true}
	m.syncViewportDims()
	if got, want := m.viewport.Height(), before-popupDialogHeight; got != want {
		t.Errorf("slash active: viewport.Height() = %d, want %d (popupDialogHeight reserved)", got, want)
	}

	m.slash = slashState{}
	m.syncViewportDims()
	if got := m.viewport.Height(); got != before {
		t.Errorf("neither active: viewport.Height() = %d, want %d (no popup reserved)", got, before)
	}
}

// TestViewPlacesPopupAboveInput covers the resolved layout: [transcript] ->
// [popup, only while active] -> [input] -> [status block]. Before issue
// #139 the popup rendered as an appended row below the input; this is a
// deliberate reordering.
func TestViewPlacesPopupAboveInput(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	m.mention = mentionState{active: true, matches: []mentionMatch{{path: "MENTIONMARKER.go"}}}

	content := m.View().Content
	popupIdx := strings.Index(content, "MENTIONMARKER.go")
	inputIdx := strings.Index(content, m.input.View())
	if popupIdx == -1 {
		t.Fatal("View() content doesn't include the active mention popup")
	}
	if inputIdx == -1 || popupIdx >= inputIdx {
		t.Errorf("popup (at %d) is not positioned above the input (at %d)", popupIdx, inputIdx)
	}
}

// TestViewCursorRowIncludesPopupDialogHeightWhenActive covers the cursor-
// offset half of the relayout: since the popup now sits between the
// viewport and the input, the input's on-screen row (and so the cursor's)
// shifts down by popupDialogHeight while a popup is active.
func TestViewCursorRowIncludesPopupDialogHeightWhenActive(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)
	m.input.Focus()
	m.input.SetVirtualCursor(false)
	m.mention = mentionState{active: true, matches: []mentionMatch{{path: "a.go"}}}
	m.syncViewportDims()

	cur := m.input.Cursor()
	if cur == nil {
		t.Fatal("input.Cursor() = nil, want a cursor (input is focused)")
	}

	v := m.View()
	want := cur.Y + m.viewport.Height() + 1 + popupDialogHeight
	if v.Cursor == nil || v.Cursor.Y != want {
		t.Errorf("View().Cursor.Y = %v, want %d (popupDialogHeight added while a popup is active)", v.Cursor, want)
	}
}
