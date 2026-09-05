package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
)

// alwaysContinue is a hand-written fake ContinuationGuard that always
// rejects the stop, for driving Loop.MaxContinuations' runaway-guard cap
// independently of any real heuristic's own judgment.
func alwaysContinue(_ []provider.Message, _ provider.DoneEvent) (string, bool) {
	return "keep going", true
}

// scriptedStops builds n scripted turns, each a bare no-tool-calls
// DoneEvent — enough to drive ShouldContinue's no-tool-calls branch
// repeatedly without any tool dispatch or streamed text muddying the
// assertions.
func scriptedStops(n int) [][]provider.Event {
	turns := make([][]provider.Event, n)
	for i := range turns {
		turns[i] = []provider.Event{provider.DoneEvent{FinishReason: "stop"}}
	}
	return turns
}

// TestRunCapsForcedContinuationsAtMaxContinuations drives issue #210's
// runaway guard: a ShouldContinue that always says "again" must not loop
// Run forever — it's capped at MaxContinuations forced continuations,
// after which Run accepts the next stop regardless of the guard's verdict.
func TestRunCapsForcedContinuationsAtMaxContinuations(t *testing.T) {
	const max = 2
	fp := &fakeProvider{turns: scriptedStops(max + 1)}
	l := Loop{Provider: fp, ShouldContinue: alwaysContinue, MaxContinuations: max}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	if _, err := l.Run(context.Background(), req, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != max+1 {
		t.Fatalf("Provider.Stream called %d times, want %d (max forced continuations + the accepted stop)", len(fp.requests), max+1)
	}
}

// TestRunCapsForcedContinuationsAtDefaultWhenUnset covers
// MaxContinuations' "<= 0 uses the default" convention (matching
// KeepRecentTurns): with MaxContinuations left unset, the cap is
// defaultMaxContinuations.
func TestRunCapsForcedContinuationsAtDefaultWhenUnset(t *testing.T) {
	fp := &fakeProvider{turns: scriptedStops(defaultMaxContinuations + 1)}
	l := Loop{Provider: fp, ShouldContinue: alwaysContinue}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	if _, err := l.Run(context.Background(), req, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != defaultMaxContinuations+1 {
		t.Fatalf("Provider.Stream called %d times, want %d (default cap + the accepted stop)", len(fp.requests), defaultMaxContinuations+1)
	}
}

// TestRunForcesExactlyOneExtraTurnAndThreadsReasonBack covers a guard that
// rejects once then accepts: Run must force exactly one extra turn, with
// the returned reason injected as a user-role Message the model's next
// turn actually sees.
func TestRunForcesExactlyOneExtraTurnAndThreadsReasonBack(t *testing.T) {
	calls := 0
	guard := func(_ []provider.Message, _ provider.DoneEvent) (string, bool) {
		calls++
		return "please keep going", calls == 1
	}
	fp := &fakeProvider{turns: [][]provider.Event{
		{provider.DoneEvent{FinishReason: "stop"}},
		{provider.TextDeltaEvent{Text: "done now"}, provider.DoneEvent{FinishReason: "stop"}},
	}}
	l := Loop{Provider: fp, ShouldContinue: guard}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	got, err := l.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != 2 {
		t.Fatalf("Provider.Stream called %d times, want 2 (one forced continuation)", len(fp.requests))
	}

	// user, injected user reason, assistant("done now")
	if len(got) != 3 {
		t.Fatalf("len(messages) = %d, want 3: %+v", len(got), got)
	}
	if got[1].Role != "user" || got[1].Content != "please keep going" {
		t.Errorf("messages[1] = %+v, want the guard's reason threaded in as a user-role message", got[1])
	}
	if got[2].Role != "assistant" || got[2].Content != "done now" {
		t.Errorf("messages[2] = %+v, want assistant %q", got[2], "done now")
	}

	secondReqMessages := fp.requests[1].Messages
	var sawReason bool
	for _, m := range secondReqMessages {
		if m.Role == "user" && m.Content == "please keep going" {
			sawReason = true
		}
	}
	if !sawReason {
		t.Errorf("second request messages = %+v, want the injected reason threaded back in", secondReqMessages)
	}
}

// TestRunNilShouldContinueLeavesNoToolCallsBehaviorUnchanged covers the
// nil-guard case: with ShouldContinue unset, Run accepts the model's own
// stop immediately, same as every no-more-tool-calls test predating issue
// #210.
func TestRunNilShouldContinueLeavesNoToolCallsBehaviorUnchanged(t *testing.T) {
	fp := &fakeProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "hello"}, provider.DoneEvent{FinishReason: "stop"}},
	}}
	l := Loop{Provider: fp}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	got, err := l.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("Provider.Stream called %d times, want 1 (no guard, no forced continuation)", len(fp.requests))
	}
	if len(got) != 2 || got[1].Role != "assistant" || got[1].Content != "hello" {
		t.Errorf("messages = %+v, want unchanged 2-message result", got)
	}
}

// TestRunAgentDoneFiresOnceEvenAfterRejectedContinuation extends
// agent_test.go's TestRunAgentDoneHookFiresOnceWithFinalPayload with a
// ShouldContinue guard that rejects the first no-tool-calls turn: agentDone
// must still fire exactly once, only on the turn that actually concludes
// the invocation, never on the turn the guard rejected.
func TestRunAgentDoneFiresOnceEvenAfterRejectedContinuation(t *testing.T) {
	dir := t.TempDir()
	outPath := dir + "/agent-done-saw.json"

	calls := 0
	guard := func(_ []provider.Message, _ provider.DoneEvent) (string, bool) {
		calls++
		return "", calls == 1 // reject the first stop, accept the second
	}
	fp := &fakeProvider{turns: [][]provider.Event{
		{provider.DoneEvent{FinishReason: "stop", ModelUsed: "intermediate/model"}},
		{provider.DoneEvent{FinishReason: "stop", ModelUsed: "final/model"}},
	}}
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		AgentDone: []config.HookConfig{{Command: "cat >> " + outPath + "; echo"}},
	}}
	l := Loop{Provider: fp, Hooks: hooks, ShouldContinue: guard}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("ShouldContinue called %d times, want 2 (once per no-tool-calls turn)", calls)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("agentDone hook did not run: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("agentDone hook ran %d times, want exactly 1: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"modelUsed":"final/model"`) {
		t.Errorf("captured stdin = %q, want the concluding (second) turn's own payload", lines[0])
	}
}

// fakeSafetyTool is fakeTool with a caller-chosen SideEffect classification,
// for exercising DefaultShouldContinue's per-Tool lookup against every
// SideEffect value rather than fakeTool's hardwired SideEffectRead.
type fakeSafetyTool struct {
	fakeTool
	sideEffect tool.SideEffect
}

func (f *fakeSafetyTool) Safety() tool.Safety {
	return tool.Safety{SideEffect: f.sideEffect}
}

// TestDefaultShouldContinueRejectsWithoutAWriteToolCall covers issue #210's
// concrete default heuristic: it rejects the stop unless some dispatched
// tool call this invocation was classified SideEffectWrite, regardless of
// which other SideEffect kinds ran.
func TestDefaultShouldContinueRejectsWithoutAWriteToolCall(t *testing.T) {
	cases := []struct {
		name      string
		sideEff   tool.SideEffect
		wantAgain bool
	}{
		{name: "read-classified tool forces another turn", sideEff: tool.SideEffectRead, wantAgain: true},
		{name: "shell-classified tool forces another turn", sideEff: tool.SideEffectShell, wantAgain: true},
		{name: "network-classified tool forces another turn", sideEff: tool.SideEffectNetwork, wantAgain: true},
		{name: "write-classified tool accepts the stop", sideEff: tool.SideEffectWrite, wantAgain: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ft := &fakeSafetyTool{fakeTool: fakeTool{name: "atool", result: tool.Result{Content: "ok"}}, sideEffect: tc.sideEff}
			registry := tool.NewRegistry(ft)
			fp := &fakeProvider{turns: [][]provider.Event{
				{
					provider.ToolCallEvent{ID: "call_1", Name: "atool", ArgsJSON: `{}`},
					provider.DoneEvent{FinishReason: "tool_calls"},
				},
				{
					provider.DoneEvent{FinishReason: "stop"},
				},
				{
					provider.TextDeltaEvent{Text: "second turn"},
					provider.DoneEvent{FinishReason: "stop"},
				},
			}}
			// MaxContinuations: 1 bounds this to exactly one forced
			// continuation regardless of the second turn's own write
			// status — DefaultShouldContinue is stateless and would keep
			// rejecting every write-less stop up to the cap otherwise,
			// which isn't what this table is exercising.
			l := Loop{Provider: fp, Tools: registry, ShouldContinue: DefaultShouldContinue(registry), MaxContinuations: 1}

			req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "do it"}}}
			if _, err := l.Run(context.Background(), req, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			gotAgain := len(fp.requests) == 3
			if gotAgain != tc.wantAgain {
				t.Errorf("Provider.Stream called %d times (forced another turn = %v), want %v", len(fp.requests), gotAgain, tc.wantAgain)
			}
		})
	}
}

// TestDefaultShouldContinueUnknownToolNameIsNotAWrite covers dispatch's own
// unknown-tool-name error path: a ToolCall naming a tool absent from the
// registry (already surfaced as an error Result by dispatch) must not be
// mistaken for a write.
func TestDefaultShouldContinueUnknownToolNameIsNotAWrite(t *testing.T) {
	registry := tool.NewRegistry() // empty: every call name is unknown
	guard := DefaultShouldContinue(registry)

	messages := []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "call_1", Name: "nonexistent"}}},
	}
	if _, again := guard(messages, provider.DoneEvent{}); !again {
		t.Error("DefaultShouldContinue accepted the stop after only an unknown-tool call, want it to force another turn")
	}
}
