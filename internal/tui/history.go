package tui

// history is the TUI's per-session Up/Down input-recall stack (issue #58):
// entries are appended on every submitted message, unbounded, and never
// persisted past the process. idx == -1 means the user is editing live (not
// currently cycling); draft holds whatever was being typed when cycling
// started, restored as a virtual "most recent" entry once Down passes the
// newest real entry.
type history struct {
	entries []string
	idx     int
	draft   string
}

func newHistory() history {
	return history{idx: -1}
}

// add appends a newly submitted message and resets any in-progress cycling.
func (h *history) add(msg string) {
	h.entries = append(h.entries, msg)
	h.idx = -1
	h.draft = ""
}

// up moves one entry older. current is the live input text, saved as the
// draft the first time cycling starts. Returns ok=false (no change) when
// there are no entries, or cycling is already at the oldest one.
func (h *history) up(current string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.idx == -1 {
		h.draft = current
		h.idx = len(h.entries) - 1
		return h.entries[h.idx], true
	}
	if h.idx == 0 {
		return "", false
	}
	h.idx--
	return h.entries[h.idx], true
}

// down moves one entry newer, restoring the saved draft once the newest
// entry is passed. Returns ok=false (no change) when not currently cycling.
func (h *history) down() (string, bool) {
	if h.idx == -1 {
		return "", false
	}
	if h.idx == len(h.entries)-1 {
		h.idx = -1
		draft := h.draft
		h.draft = ""
		return draft, true
	}
	h.idx++
	return h.entries[h.idx], true
}
