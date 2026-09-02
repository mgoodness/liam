package tui

import (
	"testing"

	"github.com/mgoodness/liam/internal/agent"
	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/provider"
)

// TestSubmitSlashCompactCondensesHistoryAndReportsSuccess covers issue
// #117's manual trigger: /compact runs the same agent.Loop.Compact path
// the automatic/reactive triggers use, against history long enough to
// actually condense, and reports success once the async compaction
// completes.
func TestSubmitSlashCompactCondensesHistoryAndReportsSuccess(t *testing.T) {
	fp := &multiCallProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "the summary"}, provider.DoneEvent{}},
	}}
	m := New(agent.Loop{Provider: fp, KeepRecentTurns: 1}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	var history []provider.Message
	for i := 0; i < 5; i++ {
		history = append(history,
			provider.Message{Role: "user", Content: "turn"},
			provider.Message{Role: "assistant", Content: "reply"},
		)
	}
	m.sess.Messages = history
	m.input.SetValue("/compact")

	next, cmd := m.submit()
	mm := next.(Model)
	if !mm.busy {
		t.Fatal("busy = false right after submit(\"/compact\"), want true")
	}
	if cmd == nil {
		t.Fatal("submit(\"/compact\") returned a nil cmd, want the async compaction command")
	}

	final := drain(t, mm, cmd)
	if final.busy {
		t.Error("busy = true after compaction finished, want false")
	}
	if len(final.sess.Messages) >= len(history) {
		t.Errorf("sess.Messages len = %d, want fewer than %d after compaction", len(final.sess.Messages), len(history))
	}
	if len(final.lines) != 2 || final.lines[0].role != "info" || final.lines[0].text != "Conversation compacted." {
		t.Fatalf("lines = %+v, want \"Conversation compacted.\" followed by the completion line", final.lines)
	}
	if final.lines[1].role != "complete" {
		t.Errorf("lines[1].role = %q, want \"complete\"", final.lines[1].role)
	}
}

// TestSubmitSlashCompactWithShortHistoryReportsNothingToCompact covers the
// no-op path: history within the sliding window compacts to nothing, and
// /compact says so rather than silently doing nothing.
func TestSubmitSlashCompactWithShortHistoryReportsNothingToCompact(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	m.sess.Messages = []provider.Message{{Role: "user", Content: "hi"}}
	m.input.SetValue("/compact")

	next, cmd := m.submit()
	final := drain(t, next.(Model), cmd)

	if final.busy {
		t.Error("busy = true after compaction finished, want false")
	}
	if len(final.lines) != 1 || final.lines[0].role != "info" || final.lines[0].text != "Nothing to compact." {
		t.Fatalf("lines = %+v, want a single \"Nothing to compact.\" info line", final.lines)
	}
}

// TestSubmitSlashCompactCanceledReportsInterrupted covers /compact's Escape
// wiring: m.cancel is set exactly like a turn's, so canceling before the
// async compaction command runs must report "[interrupted]" — not the
// misleading "Nothing to compact." a canceled-but-still-ok=false result
// would otherwise produce.
func TestSubmitSlashCompactCanceledReportsInterrupted(t *testing.T) {
	m := New(agent.Loop{}, config.Config{}, nil)
	m.indicatorTick = 0 // avoid a real 90ms sleep per tick when drain() invokes cmd() below
	m.sess.Messages = []provider.Message{{Role: "user", Content: "hi"}}
	m.input.SetValue("/compact")

	next, cmd := m.submit()
	mm := next.(Model)
	mm.cancelTurn() // Escape, before the returned cmd actually runs

	final := drain(t, mm, cmd)
	if len(final.lines) != 1 || final.lines[0].role != "system" || final.lines[0].text != "[interrupted]" {
		t.Fatalf("lines = %+v, want a single \"[interrupted]\" system line", final.lines)
	}
}
