package tui

import "testing"

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
