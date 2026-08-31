// Package compact implements ticket #54's compaction mechanism (see
// CONTEXT.md's "Compaction" glossary entry): a sliding window of the most
// recent turns kept verbatim, with everything older condensed into a
// single summary via a model-summarization call that reuses Provider.Stream
// on the conversation's own current model — see issue #33's resolution for
// the design this implements.
package compact

import (
	"context"
	"fmt"
	"strings"

	"github.com/mgoodness/liam/internal/provider"
)

// DefaultKeepRecentTurns is the sliding-window size Compact preserves
// verbatim when keepRecentTurns is <= 0. Not spec'd to a specific number
// by issue #33's resolution — 20 is a reasonable starting default, callers
// needing a different size (e.g. tests exercising the boundary) pass their
// own keepRecentTurns.
const DefaultKeepRecentTurns = 20

// activateSkillToolName must match tool.ActivateSkill.Name(). Duplicated
// here rather than importing internal/tool (which would pull in the
// tool/skill packages for one string) — a skill-activation result is
// identified structurally, by an assistant ToolCall of this Name, not by
// any tool-package type.
const activateSkillToolName = "activate_skill"

// summarizerInstructions seeds the model-summarization call, appended as a
// trailing user turn after the transcript being condensed.
const summarizerInstructions = "Summarize the conversation above so it can be continued without the full transcript. " +
	"Capture what the user is trying to accomplish, decisions made, and the current state of any in-progress work. " +
	"Be concise, but do not omit anything a continuation would need. Respond with the summary only, no preamble."

// summaryPrefix labels the synthetic message Compact inserts in place of
// the condensed turns, so it reads unambiguously as a summary rather than
// something either party actually said.
const summaryPrefix = "[Compacted summary of earlier conversation]\n\n"

// Compact condenses the portion of messages older than the most recent
// keepRecentTurns user turns into a single summary message, produced by
// one Provider.Stream call against model. Turns within the sliding window
// (keepRecentTurns <= 0 uses DefaultKeepRecentTurns) are returned
// unmodified; compacted reports whether there was anything older than the
// window to condense — false means messages is returned as-is and p is
// never called.
//
// A "tool"-role message that resulted from activating a skill (see
// internal/tool.ActivateSkill) is excluded from what's sent to the
// summarizer and, along with the assistant turn that requested it,
// re-appended verbatim immediately after the summary — so a skill's
// instructions survive compaction even though the turn that loaded them
// falls outside the sliding window.
func Compact(ctx context.Context, p provider.Provider, model string, messages []provider.Message, keepRecentTurns int) (result []provider.Message, compacted bool, err error) {
	if keepRecentTurns <= 0 {
		keepRecentTurns = DefaultKeepRecentTurns
	}

	splitIdx := splitIndex(messages, keepRecentTurns)
	if splitIdx <= 0 {
		return messages, false, nil
	}

	old, recent := messages[:splitIdx], messages[splitIdx:]
	skillTurns, forSummary := extractSkillTurns(old)

	summary, err := summarize(ctx, p, model, forSummary)
	if err != nil {
		return nil, false, fmt.Errorf("compact: summarize older turns: %w", err)
	}

	out := make([]provider.Message, 0, 1+len(skillTurns)+len(recent))
	out = append(out, provider.Message{Role: "user", Content: summaryPrefix + summary})
	out = append(out, skillTurns...)
	out = append(out, recent...)
	return out, true, nil
}

// splitIndex returns the index in messages where the most recent
// keepRecentTurns turns begin. A turn boundary is a "user"-role message —
// tool calls and their results always fall between one user message and
// the next (the agent loop's own Run threads them in that order), so
// splitting only at user-message indices never lands inside a
// tool-call/tool-result exchange. It returns 0 (nothing to compact) when
// messages holds keepRecentTurns or fewer turns.
func splitIndex(messages []provider.Message, keepRecentTurns int) int {
	var turnStarts []int
	for i, m := range messages {
		if m.Role == "user" {
			turnStarts = append(turnStarts, i)
		}
	}
	if len(turnStarts) <= keepRecentTurns {
		return 0
	}
	return turnStarts[len(turnStarts)-keepRecentTurns]
}

// extractSkillTurns scans old for activate_skill tool calls, returning
// (in original order) each such call's whole turn — the assistant message
// that requested it plus every tool-role message immediately following
// that answers one of that assistant message's ToolCalls — verbatim, for
// re-appending after the summary. forSummary is a copy of old suitable for
// the summarizer: each activate_skill call's own tool result (the skill's
// full body) is replaced with a placeholder there, so the summarizer still
// sees a well-formed transcript — every tool call still has a matching
// tool result — without the skill body consuming its input.
func extractSkillTurns(old []provider.Message) (skillTurns []provider.Message, forSummary []provider.Message) {
	forSummary = append([]provider.Message(nil), old...)

	for i, m := range old {
		if m.Role != "assistant" {
			continue
		}
		var skillCallIDs []string
		for _, c := range m.ToolCalls {
			if c.Name == activateSkillToolName {
				skillCallIDs = append(skillCallIDs, c.ID)
			}
		}
		if len(skillCallIDs) == 0 {
			continue
		}

		j := i + 1
		for j < len(old) && old[j].Role == "tool" && belongsToCall(m.ToolCalls, old[j].ToolCallID) {
			j++
		}
		skillTurns = append(skillTurns, old[i:j]...)

		for _, id := range skillCallIDs {
			for k := i + 1; k < j; k++ {
				if forSummary[k].ToolCallID == id {
					forSummary[k] = provider.Message{
						Role:       "tool",
						Content:    "[skill instructions omitted from summary — preserved separately after compaction]",
						ToolCallID: id,
					}
				}
			}
		}
	}
	return skillTurns, forSummary
}

func belongsToCall(calls []provider.ToolCall, id string) bool {
	for _, c := range calls {
		if c.ID == id {
			return true
		}
	}
	return false
}

// summarize runs one Provider.Stream call against transcript plus a
// trailing summarizerInstructions turn, and returns the accumulated text
// from the response.
func summarize(ctx context.Context, p provider.Provider, model string, transcript []provider.Message) (string, error) {
	req := provider.Request{
		Model:    model,
		Messages: append(append([]provider.Message(nil), transcript...), provider.Message{Role: "user", Content: summarizerInstructions}),
	}

	var text strings.Builder
	for ev, err := range p.Stream(ctx, req) {
		if err != nil {
			return "", err
		}
		if td, ok := ev.(provider.TextDeltaEvent); ok {
			text.WriteString(td.Text)
		}
	}
	return text.String(), nil
}
