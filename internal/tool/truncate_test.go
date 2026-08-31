package tool

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUnderCapReturnsUnchanged(t *testing.T) {
	s := "hello world"
	if got := truncate(s); got != s {
		t.Errorf("truncate(%q) = %q, want unchanged", s, got)
	}
}

func TestTruncateAtCapReturnsUnchanged(t *testing.T) {
	s := strings.Repeat("a", outputCap)
	if got := truncate(s); got != s {
		t.Errorf("truncate() at exactly outputCap chars was modified, want unchanged")
	}
}

func TestTruncateOverCapCutsAtLineBoundaryWithMarker(t *testing.T) {
	// 200 lines of 50 chars each = 10,050 bytes (well over outputCap), so the
	// cut must land on a '\n' at or before outputCap, not mid-line.
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, strings.Repeat("x", 49))
	}
	s := strings.Join(lines, "\n")

	got := truncate(s)

	if len(got) >= len(s) {
		t.Fatalf("truncate() did not shrink over-cap input: got %d bytes, input was %d", len(got), len(s))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncate() output = %q, want a truncation marker", got)
	}
	kept, _, found := strings.Cut(got, "\n[")
	if !found {
		t.Fatalf("truncate() output = %q, want the marker on its own line starting with \"[\"", got)
	}
	if !strings.HasPrefix(s, kept) {
		t.Fatalf("truncate() kept content isn't a prefix of the original input")
	}
	if !strings.HasSuffix(kept, "\n") {
		t.Errorf("truncate() cut mid-line: kept content %q doesn't end at a newline", kept)
	}
	if len(kept) > outputCap {
		t.Errorf("truncate() kept %d bytes, want at most outputCap (%d)", len(kept), outputCap)
	}
}

func TestTruncateOverCapWithNoNewlineFallsBackToHardCut(t *testing.T) {
	s := strings.Repeat("a", outputCap*2)

	got := truncate(s)

	if !strings.HasPrefix(got, strings.Repeat("a", outputCap)) {
		t.Errorf("truncate() with no newline should hard-cut at outputCap")
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncate() output = %q, want a truncation marker", got)
	}
}

// TestTruncateNeverSplitsAMultiByteRune covers the hard-cut fallback's edge
// case: outputCap can land in the middle of a multi-byte UTF-8 sequence
// (e.g. an emoji) when there's no newline nearby to save it. truncate must
// back up to the previous rune boundary rather than emit invalid UTF-8.
func TestTruncateNeverSplitsAMultiByteRune(t *testing.T) {
	// outputCap-1 ASCII bytes (one short of the cap) followed by a 4-byte
	// emoji straddling the cap, with no newline anywhere to redirect the
	// cut — this forces the hard-cut fallback right through the rune.
	s := strings.Repeat("a", outputCap-1) + "🎉" + strings.Repeat("b", 100)

	got := truncate(s)

	kept, _, found := strings.Cut(got, "\n[")
	if !found {
		t.Fatalf("truncate() output = %q, want a truncation marker", got)
	}
	if !utf8.ValidString(kept) {
		t.Fatalf("truncate() kept content is not valid UTF-8: %q", kept)
	}
	if !strings.HasPrefix(s, kept) {
		t.Fatalf("truncate() kept content isn't a prefix of the original input")
	}
}
