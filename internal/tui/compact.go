package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/mgoodness/liam/internal/provider"
)

// compactDoneMsg reports /compact's completion: agent.Loop.Compact's
// return value, threaded back through a plain tea.Cmd (unlike turnDoneMsg,
// there's no streamed provider.Event sequence to forward along the way, so
// no events channel is needed here). canceled is set when Escape fired
// mid-compaction (see compact()'s ctx wiring) — checked ahead of ok so an
// interrupted call, which also reports ok=false, renders as "[interrupted]"
// rather than the misleading "Nothing to compact."
type compactDoneMsg struct {
	messages []provider.Message
	ok       bool
	canceled bool
}

// compact runs /compact — issue #117's manual counterpart to the
// automatic (~85% usage) and reactive (ContextTooLong) triggers already
// wired into agent.Loop.Run, invoking the exact same agent.Loop.Compact
// path on demand rather than waiting for either to fire on its own. Like
// submit()'s own turn dispatch, the actual compaction (a Provider.Stream
// summarization call) runs off the main Update loop — here via the
// returned tea.Cmd's own async execution — rather than blocking it; m.busy
// guards against a turn or another /compact starting concurrently, and
// m.cancel is wired the same way a turn's is, so Escape can interrupt a
// slow summarization call rather than leaving the user stuck waiting.
func (m Model) compact() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.busy = true
	m.cancel = cancel
	loop := m.loop
	messages := append([]provider.Message(nil), m.sess.Messages...)
	model := m.reqModel
	return m, func() tea.Msg {
		compacted, ok := loop.Compact(ctx, messages, model)
		return compactDoneMsg{messages: compacted, ok: ok, canceled: ctx.Err() != nil}
	}
}
