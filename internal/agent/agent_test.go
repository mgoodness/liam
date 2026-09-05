package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mgoodness/liam/internal/config"
	"github.com/mgoodness/liam/internal/hook"
	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/session"
	"github.com/mgoodness/liam/internal/skill"
	"github.com/mgoodness/liam/internal/tool"
)

// fakeProvider scripts one []provider.Event per call to Stream, in call
// order — call N of a multi-attempt/multi-turn agent loop gets turns[N]
// (each retry attempt is its own Stream call, indexed the same way as a
// distinct turn). errs, indexed the same way, optionally yields an error
// after that call's events — e.g. a scripted *provider.ProviderError to
// drive the retry policy, or a plain error for a non-retryable failure. It
// also records every Request it was called with, so tests can assert on
// how Run threaded history back in, and how many attempts it made.
type fakeProvider struct {
	turns [][]provider.Event
	errs  []error

	requests []provider.Request
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Stream(_ context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	idx := len(f.requests)
	f.requests = append(f.requests, req)

	return func(yield func(provider.Event, error) bool) {
		if idx >= len(f.turns) && idx >= len(f.errs) {
			yield(nil, errors.New("fakeProvider: no scripted turn for this call"))
			return
		}
		if idx < len(f.turns) {
			for _, ev := range f.turns[idx] {
				if !yield(ev, nil) {
					return
				}
			}
		}
		if idx < len(f.errs) && f.errs[idx] != nil {
			yield(nil, f.errs[idx])
		}
	}
}

// fakeTool records the args it was called with and returns a scripted
// Result.
type fakeTool struct {
	name   string
	result tool.Result
	gotArg map[string]any
}

func (f *fakeTool) Name() string            { return f.name }
func (f *fakeTool) Description() string     { return "fake tool" }
func (f *fakeTool) Parameters() tool.Schema { return tool.Schema{"type": "object"} }
func (f *fakeTool) Safety() tool.Safety {
	return tool.Safety{SideEffect: tool.SideEffectRead}
}
func (f *fakeTool) Run(_ context.Context, args map[string]any) tool.Result {
	f.gotArg = args
	return f.result
}

// cancelingTool calls cancel as part of Run, simulating an Escape-cancelled
// context.Context arriving mid-tool-call.
type cancelingTool struct {
	name   string
	cancel context.CancelFunc
	result tool.Result
}

func (c *cancelingTool) Name() string            { return c.name }
func (c *cancelingTool) Description() string     { return "canceling tool" }
func (c *cancelingTool) Parameters() tool.Schema { return tool.Schema{"type": "object"} }
func (c *cancelingTool) Safety() tool.Safety {
	return tool.Safety{SideEffect: tool.SideEffectRead}
}
func (c *cancelingTool) Run(context.Context, map[string]any) tool.Result {
	c.cancel()
	return c.result
}

func TestRunNoToolCallsReturnsAssistantMessage(t *testing.T) {
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.TextDeltaEvent{Text: "hel"},
			provider.TextDeltaEvent{Text: "lo"},
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	l := Loop{Provider: fp}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	got, err := l.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(fp.requests) != 1 {
		t.Fatalf("Provider.Stream called %d times, want 1", len(fp.requests))
	}
	if len(got) != 2 {
		t.Fatalf("len(messages) = %d, want 2: %+v", len(got), got)
	}
	if got[1].Role != "assistant" || got[1].Content != "hello" {
		t.Errorf("messages[1] = %+v, want assistant %q", got[1], "hello")
	}
}

func TestRunDispatchesToolCallAndThreadsResultBack(t *testing.T) {
	ft := &fakeTool{name: "read", result: tool.Result{Content: "file content"}}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "read", ArgsJSON: `{"path":"foo"}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{
			provider.TextDeltaEvent{Text: "done"},
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ft)}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "read foo"}}}
	got, err := l.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(fp.requests) != 2 {
		t.Fatalf("Provider.Stream called %d times, want 2", len(fp.requests))
	}
	if want := map[string]any{"path": "foo"}; ft.gotArg["path"] != want["path"] {
		t.Errorf("tool got args = %+v, want %+v", ft.gotArg, want)
	}

	// user, assistant(tool_calls), tool, assistant("done")
	if len(got) != 4 {
		t.Fatalf("len(messages) = %d, want 4: %+v", len(got), got)
	}
	assistantCall := got[1]
	if assistantCall.Role != "assistant" || len(assistantCall.ToolCalls) != 1 || assistantCall.ToolCalls[0].ID != "call_1" {
		t.Errorf("messages[1] = %+v, want assistant with ToolCalls[0].ID = call_1", assistantCall)
	}
	toolResult := got[2]
	if toolResult.Role != "tool" || toolResult.Content != "file content" || toolResult.ToolCallID != "call_1" {
		t.Errorf("messages[2] = %+v, want tool result threaded back with ToolCallID = call_1", toolResult)
	}
	if got[3].Role != "assistant" || got[3].Content != "done" {
		t.Errorf("messages[3] = %+v, want assistant %q", got[3], "done")
	}

	// The second request must carry the tool result forward so the
	// provider can see it.
	secondReqMessages := fp.requests[1].Messages
	if len(secondReqMessages) != 3 {
		t.Fatalf("second request has %d messages, want 3: %+v", len(secondReqMessages), secondReqMessages)
	}
	if secondReqMessages[2].ToolCallID != "call_1" {
		t.Errorf("second request's tool message ToolCallID = %q, want call_1", secondReqMessages[2].ToolCallID)
	}
}

func TestRunUnknownToolReportsErrorWithoutFailingLoop(t *testing.T) {
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "nonexistent", ArgsJSON: `{}`},
			provider.DoneEvent{},
		},
		{
			provider.DoneEvent{},
		},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry()}

	got, err := l.Run(context.Background(), provider.Request{}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var toolMsg *provider.Message
	for i := range got {
		if got[i].Role == "tool" {
			toolMsg = &got[i]
		}
	}
	if toolMsg == nil {
		t.Fatalf("no tool-role message in %+v", got)
	}
	if toolMsg.Content == "" {
		t.Errorf("tool message Content is empty, want an error description")
	}
}

func TestRunInvalidArgsJSONReportsErrorWithoutFailingLoop(t *testing.T) {
	ft := &fakeTool{name: "read"}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "read", ArgsJSON: `not json`},
			provider.DoneEvent{},
		},
		{
			provider.DoneEvent{},
		},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ft)}

	got, err := l.Run(context.Background(), provider.Request{}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if ft.gotArg != nil {
		t.Errorf("tool.Run was called with malformed args, want dispatch to short-circuit before calling it")
	}

	var toolMsg *provider.Message
	for i := range got {
		if got[i].Role == "tool" {
			toolMsg = &got[i]
		}
	}
	if toolMsg == nil || toolMsg.Content == "" {
		t.Fatalf("expected a non-empty tool-role error message, got %+v", got)
	}
}

func TestRunPropagatesProviderError(t *testing.T) {
	wantErr := errors.New("boom")
	fp := &fakeProvider{
		turns: [][]provider.Event{{provider.TextDeltaEvent{Text: "partial"}}},
		errs:  []error{wantErr},
	}
	l := Loop{Provider: fp}

	_, err := l.Run(context.Background(), provider.Request{}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
}

func TestRunInvokesOnEventForEveryEvent(t *testing.T) {
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.TextDeltaEvent{Text: "hi"},
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	l := Loop{Provider: fp}

	var seen []provider.Event
	_, err := l.Run(context.Background(), provider.Request{}, func(ev provider.Event) {
		seen = append(seen, ev)
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("onEvent called %d times, want 2: %+v", len(seen), seen)
	}
}

func TestRunAdvertisesRegisteredToolsSortedByName(t *testing.T) {
	fp := &fakeProvider{turns: [][]provider.Event{{provider.DoneEvent{}}}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(
		&fakeTool{name: "write"},
		&fakeTool{name: "bash"},
		&fakeTool{name: "edit"},
	)}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	tools := fp.requests[0].Tools
	if len(tools) != 3 {
		t.Fatalf("len(Tools) = %d, want 3", len(tools))
	}
	got := []string{tools[0].Name, tools[1].Name, tools[2].Name}
	want := []string{"bash", "edit", "write"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Tools[%d].Name = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestRunActivatesSkillAndThreadsBodyBack drives issue #53's
// activate_skill tool through the real agent loop dispatch (via the
// fake-Provider seam): the model calls activate_skill("commit-messages"),
// and the skill's full SKILL.md body (frontmatter already stripped by
// skill.Discover) is threaded back in as the tool result, then the model
// sees it on its next turn.
func TestRunActivatesSkillAndThreadsBodyBack(t *testing.T) {
	catalog := []skill.Skill{
		{Name: "commit-messages", Description: "Write conventional commit messages.", Body: "# commit-messages\n\nUse Conventional Commits."},
	}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "activate_skill", ArgsJSON: `{"name":"commit-messages"}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{
			provider.TextDeltaEvent{Text: "done"},
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(tool.ActivateSkill{Catalog: catalog})}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "write me a commit message"}}}
	got, err := l.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var toolMsg *provider.Message
	for i := range got {
		if got[i].Role == "tool" {
			toolMsg = &got[i]
		}
	}
	if toolMsg == nil {
		t.Fatalf("no tool-role message in %+v", got)
	}
	if toolMsg.Content != "# commit-messages\n\nUse Conventional Commits." {
		t.Errorf("tool result Content = %q, want the skill's body", toolMsg.Content)
	}

	// The second request must carry the activated skill's body forward so
	// the model actually sees it.
	secondReqMessages := fp.requests[1].Messages
	var sawBody bool
	for _, m := range secondReqMessages {
		if m.Role == "tool" && m.Content == "# commit-messages\n\nUse Conventional Commits." {
			sawBody = true
		}
	}
	if !sawBody {
		t.Errorf("second request messages = %+v, want the activated skill's body threaded back in", secondReqMessages)
	}
}

func TestRunPreservesPartialTextOnStreamError(t *testing.T) {
	wantErr := errors.New("boom")
	fp := &fakeProvider{
		turns: [][]provider.Event{{provider.TextDeltaEvent{Text: "partial"}}},
		errs:  []error{wantErr},
	}
	l := Loop{Provider: fp}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	got, err := l.Run(context.Background(), req, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}

	if len(got) != 2 {
		t.Fatalf("len(messages) = %d, want 2 (user + partial assistant): %+v", len(got), got)
	}
	if got[1].Role != "assistant" || got[1].Content != "partial" {
		t.Errorf("messages[1] = %+v, want assistant %q", got[1], "partial")
	}
}

// TestRunAfterToolHookRunsOnceToolCompletes exercises the afterTool
// lifecycle point end-to-end: a stub hook command writes to a temp file, and
// the test asserts it ran only after (and because of) the dispatched tool
// call completing.
func TestRunAfterToolHookRunsOnceToolCompletes(t *testing.T) {
	dir := t.TempDir()
	ranPath := dir + "/after-tool-ran"

	ft := &fakeTool{name: "bash", result: tool.Result{Content: "done"}}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "bash", ArgsJSON: `{}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
		{
			provider.DoneEvent{FinishReason: "stop"},
		},
	}}
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		AfterTool: []config.HookConfig{{Command: "touch " + ranPath}},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ft), Hooks: hooks}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if _, err := os.Stat(ranPath); err != nil {
		t.Errorf("afterTool hook did not run: %v", err)
	}
}

// TestRunAgentDoneHookFiresOnceWithFinalPayload exercises the agentDone
// lifecycle point end-to-end: across a multi-turn Run (one tool-calling turn
// followed by the concluding turn), the hook must run exactly once — not
// once per streamTurn call — carrying the concluding turn's own
// FinishReason/ModelUsed/Usage, not the intermediate tool_calls turn's.
func TestRunAgentDoneHookFiresOnceWithFinalPayload(t *testing.T) {
	dir := t.TempDir()
	outPath := dir + "/agent-done-saw.json"

	ft := &fakeTool{name: "bash", result: tool.Result{Content: "done"}}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "bash", ArgsJSON: `{}`},
			provider.DoneEvent{FinishReason: "tool_calls", ModelUsed: "intermediate/model"},
		},
		{
			provider.TextDeltaEvent{Text: "all done"},
			provider.DoneEvent{FinishReason: "stop", ModelUsed: "final/model", Usage: provider.Usage{OutputTokens: 7}},
		},
	}}
	hooks := &hook.Runner{Hooks: config.HooksConfig{
		AgentDone: []config.HookConfig{{Command: "cat >> " + outPath + "; echo"}},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ft), Hooks: hooks}

	if _, err := l.Run(context.Background(), provider.Request{}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("agentDone hook did not run: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("agentDone hook ran %d times, want exactly 1: %q", len(lines), lines)
	}

	var stdin struct {
		Lifecycle string `json:"lifecycle"`
		Done      struct {
			FinishReason string         `json:"finishReason"`
			ModelUsed    string         `json:"modelUsed"`
			Usage        provider.Usage `json:"usage"`
		} `json:"done"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &stdin); err != nil {
		t.Fatalf("unmarshaling captured stdin %q: %v", lines[0], err)
	}
	if stdin.Lifecycle != "agentDone" {
		t.Errorf("stdin lifecycle = %q, want agentDone", stdin.Lifecycle)
	}
	if stdin.Done.FinishReason != "stop" || stdin.Done.ModelUsed != "final/model" || stdin.Done.Usage.OutputTokens != 7 {
		t.Errorf("stdin.done = %+v, want the concluding turn's own payload (stop/final/model/7)", stdin.Done)
	}
}

// TestRunReturnsImmediatelyWhenCanceledDuringToolRun exercises
// Escape-cancellation mid-Tool.Run: the loop must notice ctx is canceled
// right after dispatch, not loop back into another (doomed) Provider.Stream
// call before reporting the cancellation.
func TestRunReturnsImmediatelyWhenCanceledDuringToolRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ct := &cancelingTool{name: "bash", cancel: cancel, result: tool.Result{Content: "killed"}}
	fp := &fakeProvider{turns: [][]provider.Event{
		{
			provider.ToolCallEvent{ID: "call_1", Name: "bash", ArgsJSON: `{}`},
			provider.DoneEvent{FinishReason: "tool_calls"},
		},
	}}
	l := Loop{Provider: fp, Tools: tool.NewRegistry(ct)}

	got, err := l.Run(ctx, provider.Request{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("Provider.Stream called %d times, want 1 (no doomed post-cancellation call)", len(fp.requests))
	}

	var toolMsg *provider.Message
	for i := range got {
		if got[i].Role == "tool" {
			toolMsg = &got[i]
		}
	}
	if toolMsg == nil || toolMsg.Content != "killed" {
		t.Fatalf("messages = %+v, want the tool's result preserved", got)
	}
}

// TestRunRetriesRetryableProviderErrorThenSucceeds drives issue #51's retry
// policy: a RateLimited ProviderError on the first attempt auto-retries,
// invisibly to the model — the conversation ends up with only the
// successful retry's assistant message, no trace of the failed attempt.
func TestRunRetriesRetryableProviderErrorThenSucceeds(t *testing.T) {
	fp := &fakeProvider{
		turns: [][]provider.Event{
			nil,
			{
				provider.TextDeltaEvent{Text: "hello"},
				provider.DoneEvent{FinishReason: "stop"},
			},
		},
		errs: []error{
			&provider.ProviderError{Kind: provider.ErrorKindRateLimited, Cause: errors.New("rate limited")},
		},
	}
	l := Loop{Provider: fp, Backoff: func(int) time.Duration { return 0 }}

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	got, err := l.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != 2 {
		t.Fatalf("Provider.Stream called %d times, want 2 (1 failed attempt + 1 retry)", len(fp.requests))
	}
	if len(got) != 2 {
		t.Fatalf("len(messages) = %d, want 2 (user + the retry's assistant message only): %+v", len(got), got)
	}
	if got[1].Role != "assistant" || got[1].Content != "hello" {
		t.Errorf("messages[1] = %+v, want assistant %q", got[1], "hello")
	}
}

// TestRunRetriesUpToMaxAttemptsThenFails covers a RateLimited/Unavailable
// error that never clears: the loop retries up to maxStreamAttempts total
// attempts, then surfaces the final failure.
func TestRunRetriesUpToMaxAttemptsThenFails(t *testing.T) {
	wantErr := &provider.ProviderError{Kind: provider.ErrorKindUnavailable, Cause: errors.New("down")}
	fp := &fakeProvider{errs: []error{wantErr, wantErr, wantErr}}
	l := Loop{Provider: fp, Backoff: func(int) time.Duration { return 0 }}

	_, err := l.Run(context.Background(), provider.Request{}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if len(fp.requests) != maxStreamAttempts {
		t.Fatalf("Provider.Stream called %d times, want %d (max attempts)", len(fp.requests), maxStreamAttempts)
	}
}

// TestRunDoesNotRetryNonRetryableProviderErrorKinds covers the policy's
// other branch: InvalidRequest and Unknown surface immediately by spec,
// with no backoff-and-resend retry. ContextTooLong is covered separately
// below (TestRunContextTooLong*) — issue #54 gave it its own
// compact-then-retry-once handling instead of the backoff policy tested
// here.
func TestRunDoesNotRetryNonRetryableProviderErrorKinds(t *testing.T) {
	kinds := []provider.ErrorKind{
		provider.ErrorKindInvalidRequest,
		provider.ErrorKindUnknown,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			wantErr := &provider.ProviderError{Kind: kind, Cause: errors.New("boom")}
			fp := &fakeProvider{errs: []error{wantErr}}
			l := Loop{Provider: fp, Backoff: func(int) time.Duration { return 0 }}

			_, err := l.Run(context.Background(), provider.Request{}, nil)
			if !errors.Is(err, wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, wantErr)
			}
			if len(fp.requests) != 1 {
				t.Fatalf("Provider.Stream called %d times, want 1 (no retry for Kind=%s)", len(fp.requests), kind)
			}
		})
	}
}

// fakeContextLookup is a session.ContextLookup returning a canned max
// context length per model id, for driving auto-compaction's threshold
// check without a real API call.
type fakeContextLookup struct {
	maxByModel map[string]int
}

func (f *fakeContextLookup) MaxContextLength(_ context.Context, model string) (int, error) {
	return f.maxByModel[model], nil
}

// turnsOfHistory builds n user/assistant turn pairs, for tests that need a
// message history longer than a small KeepRecentTurns window.
func turnsOfHistory(n int) []provider.Message {
	var out []provider.Message
	for i := 0; i < n; i++ {
		out = append(out,
			provider.Message{Role: "user", Content: fmt.Sprintf("turn %d", i)},
			provider.Message{Role: "assistant", Content: fmt.Sprintf("reply %d", i)},
		)
	}
	return out
}

// TestRunContextTooLongWithNothingToCompactSurfacesImmediately covers the
// degenerate case: a ContextTooLong failure on a request with no history
// older than the sliding window has nothing to compact away, so Run's one
// retry attempt is skipped and the original error surfaces, same as
// before issue #54.
func TestRunContextTooLongWithNothingToCompactSurfacesImmediately(t *testing.T) {
	wantErr := &provider.ProviderError{Kind: provider.ErrorKindContextTooLong, Cause: errors.New("too long")}
	fp := &fakeProvider{errs: []error{wantErr}}
	l := Loop{Provider: fp, Backoff: func(int) time.Duration { return 0 }}

	_, err := l.Run(context.Background(), provider.Request{}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("Provider.Stream called %d times, want 1 (nothing to compact, no retry)", len(fp.requests))
	}
}

// TestRunContextTooLongCompactsThenRetriesOnceAndSucceeds drives issue
// #54's ContextTooLong extension point end-to-end: the first attempt fails
// with ContextTooLong, Run compacts the oversized history via a
// Provider.Stream summarization call, then retries the original request
// once against the now-compacted history and succeeds.
func TestRunContextTooLongCompactsThenRetriesOnceAndSucceeds(t *testing.T) {
	history := turnsOfHistory(5) // 5 user turns; KeepRecentTurns keeps only 1
	fp := &fakeProvider{
		turns: [][]provider.Event{
			nil, // attempt 1: the too-long request, no events before the error
			{provider.TextDeltaEvent{Text: "the summary"}},                                     // the summarization call
			{provider.TextDeltaEvent{Text: "hello"}, provider.DoneEvent{FinishReason: "stop"}}, // the retried request
		},
		errs: []error{
			&provider.ProviderError{Kind: provider.ErrorKindContextTooLong, Cause: errors.New("too long")},
		},
	}
	l := Loop{Provider: fp, KeepRecentTurns: 1, Backoff: func(int) time.Duration { return 0 }}

	req := provider.Request{Messages: history}
	got, err := l.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != 3 {
		t.Fatalf("Provider.Stream called %d times, want 3 (failed attempt + summarize + retry): %+v", len(fp.requests), fp.requests)
	}

	retriedMessages := fp.requests[2].Messages
	if retriedMessages[0].Role != "user" || !strings.Contains(retriedMessages[0].Content, "the summary") {
		t.Errorf("retried request's first message = %+v, want the compacted summary", retriedMessages[0])
	}

	if got[len(got)-1].Role != "assistant" || got[len(got)-1].Content != "hello" {
		t.Errorf("final message = %+v, want the retried turn's assistant reply", got[len(got)-1])
	}
}

// TestRunContextTooLongRetryStillFailsSurfacesThatError covers the "retry
// once" half of the policy: when the post-compaction retry also fails, Run
// gives up rather than compacting again.
func TestRunContextTooLongRetryStillFailsSurfacesThatError(t *testing.T) {
	history := turnsOfHistory(5)
	retryErr := errors.New("still broken")
	fp := &fakeProvider{
		turns: [][]provider.Event{
			nil,
			{provider.TextDeltaEvent{Text: "the summary"}},
			nil,
		},
		errs: []error{
			&provider.ProviderError{Kind: provider.ErrorKindContextTooLong, Cause: errors.New("too long")},
			nil,
			retryErr,
		},
	}
	l := Loop{Provider: fp, KeepRecentTurns: 1, Backoff: func(int) time.Duration { return 0 }}

	_, err := l.Run(context.Background(), provider.Request{Messages: history}, nil)
	if !errors.Is(err, retryErr) {
		t.Fatalf("Run() error = %v, want %v", err, retryErr)
	}
	if len(fp.requests) != 3 {
		t.Fatalf("Provider.Stream called %d times, want 3 (no second compaction attempt): %+v", len(fp.requests), fp.requests)
	}
}

// TestRunAutoCompactsWhenContextPercentAtOrAboveThreshold drives issue
// #54's proactive auto-trigger: Session/ContextLookup report usage already
// at the ~85% threshold before the turn even starts, so Run compacts the
// history first and resets Session's tracker, rather than waiting for a
// ContextTooLong failure.
func TestRunAutoCompactsWhenContextPercentAtOrAboveThreshold(t *testing.T) {
	history := turnsOfHistory(5)
	sess := session.New()
	sess.Record("test-model", provider.Usage{InputTokens: 850})
	lookup := &fakeContextLookup{maxByModel: map[string]int{"test-model": 1000}} // 85.0%

	fp := &fakeProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "the summary"}},
		{provider.TextDeltaEvent{Text: "hello"}, provider.DoneEvent{FinishReason: "stop"}},
	}}
	l := Loop{Provider: fp, Session: sess, ContextLookup: lookup, KeepRecentTurns: 1}

	req := provider.Request{Model: "test-model", Messages: history}
	if _, err := l.Run(context.Background(), req, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != 2 {
		t.Fatalf("Provider.Stream called %d times, want 2 (auto-compact summarize + the turn): %+v", len(fp.requests), fp.requests)
	}

	turnMessages := fp.requests[1].Messages
	if turnMessages[0].Role != "user" || !strings.Contains(turnMessages[0].Content, "the summary") {
		t.Errorf("turn request's first message = %+v, want the compacted summary", turnMessages[0])
	}

	// Compaction must reset the tracker immediately (issue #54's criterion):
	// with onEvent nil here, nothing else touches Session afterward, so its
	// state should still show the reset, not the pre-compaction 85%.
	if sess.LastModel != "" || sess.LastContextTokens != 0 {
		t.Errorf("Session = {LastModel: %q, LastContextTokens: %d}, want both cleared by compaction", sess.LastModel, sess.LastContextTokens)
	}
}

// TestRunAutoCompactDisabledWithoutSessionOrContextLookup covers the nil
// guard: with neither Session nor ContextLookup set (headless mode's
// default), Run never consults auto-compaction at all, regardless of
// history length.
func TestRunAutoCompactDisabledWithoutSessionOrContextLookup(t *testing.T) {
	history := turnsOfHistory(5)
	fp := &fakeProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "hello"}, provider.DoneEvent{FinishReason: "stop"}},
	}}
	l := Loop{Provider: fp, KeepRecentTurns: 1}

	if _, err := l.Run(context.Background(), provider.Request{Messages: history}, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("Provider.Stream called %d times, want 1 (no auto-compaction call)", len(fp.requests))
	}
}

// TestRunRetryBackoffCanceledReturnsContextCanceled covers Escape-cancellation
// while a retry is waiting on backoff: the wait must abort immediately
// rather than sleeping out its full delay.
func TestRunRetryBackoffCanceledReturnsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fp := &fakeProvider{
		errs: []error{&provider.ProviderError{Kind: provider.ErrorKindRateLimited, Cause: errors.New("rate limited")}},
	}
	l := Loop{Provider: fp, Backoff: func(int) time.Duration {
		cancel()
		// Long enough that the test would hang if cancellation weren't honored.
		return time.Hour
	}}

	_, err := l.Run(ctx, provider.Request{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("Provider.Stream called %d times, want 1 (canceled during backoff, before the retry attempt)", len(fp.requests))
	}
}
