package tui

import "testing"

func TestHistoryUpWithNoEntriesIsNoOp(t *testing.T) {
	h := newHistory()
	if _, ok := h.up("draft"); ok {
		t.Error("up() on empty history returned ok=true, want false")
	}
}

func TestHistoryUpCyclesOldestToNewestAndSavesDraft(t *testing.T) {
	h := newHistory()
	h.add("first")
	h.add("second")

	got, ok := h.up("in progress")
	if !ok || got != "second" {
		t.Fatalf("up() = %q, %v, want %q, true", got, ok, "second")
	}
	got, ok = h.up("in progress")
	if !ok || got != "first" {
		t.Fatalf("up() = %q, %v, want %q, true", got, ok, "first")
	}
	// Already at the oldest entry: a further up() is a no-op.
	if _, ok := h.up("in progress"); ok {
		t.Error("up() past the oldest entry returned ok=true, want false")
	}
}

func TestHistoryDownWithoutCyclingIsNoOp(t *testing.T) {
	h := newHistory()
	h.add("first")

	if _, ok := h.down(); ok {
		t.Error("down() before any up() returned ok=true, want false")
	}
}

func TestHistoryDownRestoresDraftPastNewestEntry(t *testing.T) {
	h := newHistory()
	h.add("first")
	h.add("second")

	if _, ok := h.up("my draft"); !ok {
		t.Fatal("up() returned ok=false")
	}

	got, ok := h.down()
	if !ok || got != "my draft" {
		t.Fatalf("down() = %q, %v, want the preserved draft %q, true", got, ok, "my draft")
	}
	// The draft was consumed; a further down() with no cycling in progress
	// is a no-op again.
	if _, ok := h.down(); ok {
		t.Error("down() after the draft was restored returned ok=true, want false")
	}
}

func TestHistoryAddDuringCyclingResetsPosition(t *testing.T) {
	h := newHistory()
	h.add("first")
	h.up("draft")

	h.add("second")

	got, ok := h.up("draft again")
	if !ok || got != "second" {
		t.Fatalf("up() after add() = %q, %v, want the newly added entry %q, true", got, ok, "second")
	}
}

func TestHistoryIsUnbounded(t *testing.T) {
	h := newHistory()
	const n = 5000
	for i := 0; i < n; i++ {
		h.add(string(rune('a' + i%26)))
	}
	if len(h.entries) != n {
		t.Fatalf("len(entries) = %d, want %d (unbounded)", len(h.entries), n)
	}
}
