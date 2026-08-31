package agent

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/tool"
)

// fakeProvider scripts one []provider.Event per call to Stream, in call
// order — turn N of a multi-turn agent loop gets turns[N]. It also records
// every Request it was called with, so tests can assert on how the loop
// threaded history back in.
type fakeProvider struct {
	turns [][]provider.Event
	err   error // returned (once, on the final scripted turn) instead of a normal finish, if set

	requests []provider.Request
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Stream(_ context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	idx := len(f.requests)
	f.requests = append(f.requests, req)

	return func(yield func(provider.Event, error) bool) {
		if idx >= len(f.turns) {
			yield(nil, errors.New("fakeProvider: no scripted turn for this call"))
			return
		}
		for _, ev := range f.turns[idx] {
			if !yield(ev, nil) {
				return
			}
		}
		if f.err != nil && idx == len(f.turns)-1 {
			yield(nil, f.err)
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
		err:   wantErr,
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
