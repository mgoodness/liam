package agent

import (
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
)

// defaultMaxContinuations is Loop.MaxContinuations' fallback when unset,
// matching KeepRecentTurns' own "<= 0 means default" convention.
const defaultMaxContinuations = 3

// ContinuationGuard is consulted in Run's no-more-tool-calls branch — the
// point where the model's own turn produced no further tool calls and Run
// would otherwise return — before Run accepts that as the invocation's
// conclusion (issue #210). messages is everything Run has appended during
// this invocation only (never req.Messages, and unaffected by any
// mid-invocation compaction of Run's own returned history); done is the
// concluding turn's own DoneEvent. Returning again=true rejects the stop:
// Run injects reason (when non-empty) as a synthetic user-role Message so
// the model sees why it's being asked to continue, then starts another
// turn instead of returning. Loop.MaxContinuations bounds how many times
// Run honors again=true regardless of what a guard keeps returning, so a
// guard doesn't need to track its own call count to avoid looping forever.
type ContinuationGuard func(messages []provider.Message, done provider.DoneEvent) (reason string, again bool)

// maxContinuations returns l.MaxContinuations if positive, else
// defaultMaxContinuations.
func (l Loop) maxContinuations() int {
	if l.MaxContinuations > 0 {
		return l.MaxContinuations
	}
	return defaultMaxContinuations
}

// DefaultShouldContinue returns liam's built-in ContinuationGuard, wired at
// the real (non-test) Loop construction site (cmd/liam/main.go) for both
// interactive and headless modes alike. It rejects the model's own
// decision to stop whenever no tool call dispatched during the invocation
// was classified as SideEffectWrite — via each Tool's own Safety(), looked
// up in tools rather than matched by literal name, so a future
// write-classified tool is covered automatically. This is issue #210's
// concrete default heuristic: the motivating bug was liam reporting a task
// complete after a burst of pure investigation with no write/edit tool
// ever run.
func DefaultShouldContinue(tools tool.Registry) ContinuationGuard {
	return func(messages []provider.Message, _ provider.DoneEvent) (string, bool) {
		if sawWriteToolCall(tools, messages) {
			return "", false
		}
		return "You stopped without running any write/edit tool call this turn. " +
			"If the task isn't fully finished yet, keep going until it is.", true
	}
}

// sawWriteToolCall reports whether messages contains a ToolCall naming a
// Tool classified SideEffectWrite in tools. An unrecognized tool name
// (already surfaced to the model as dispatch's own error Result) counts as
// not a write.
func sawWriteToolCall(tools tool.Registry, messages []provider.Message) bool {
	for _, m := range messages {
		for _, call := range m.ToolCalls {
			if t, ok := tools[call.Name]; ok && t.Safety().SideEffect == tool.SideEffectWrite {
				return true
			}
		}
	}
	return false
}
