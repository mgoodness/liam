package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mgoodness/liam/agent"
	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// fakeProvider returns a scripted sequence of Responses, one per call to
// Complete, regardless of the messages passed in.
type fakeProvider struct {
	responses []provider.Response
	calls     int
}

func (f *fakeProvider) Complete(ctx context.Context, messages []provider.Message) (provider.Response, error) {
	if f.calls >= len(f.responses) {
		panic("fakeProvider: more calls than scripted responses")
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

// blockingProvider's Complete blocks until ctx is cancelled, simulating a
// provider request in flight when the caller cancels — then returns
// ctx.Err(), the way a real ctx-aware HTTP call would. started, if non-nil,
// is closed once Complete has been entered, so a test can safely cancel
// only after the blocking call is actually underway.
type blockingProvider struct {
	started chan struct{}
}

func (b *blockingProvider) Complete(ctx context.Context, messages []provider.Message) (provider.Response, error) {
	if b.started != nil {
		close(b.started)
	}
	<-ctx.Done()
	return provider.Response{}, ctx.Err()
}

// blockingTool builds a tool.Tool whose Handler blocks until ctx is
// cancelled and then returns ctx.Err(), simulating a tool call in flight
// when the caller cancels. started, if non-nil, is closed once the Handler
// has been entered.
func blockingTool(started chan struct{}) tool.Tool {
	return tool.Tool{
		Definition: tool.Definition{Name: "block"},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			if started != nil {
				close(started)
			}
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
}

// waitDone waits for done to close, failing the test if Run doesn't return
// promptly after cancellation.
func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly after context cancellation")
	}
}

func TestRun_NoToolCalls_ReturnsFinalText(t *testing.T) {
	p := &fakeProvider{
		responses: []provider.Response{
			{Text: "hello there"},
		},
	}
	history := []provider.Message{
		{Role: provider.RoleUser, Content: "hi"},
	}

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if text != "hello there" {
		t.Errorf("text = %q, want %q", text, "hello there")
	}
	if len(updated) != 2 {
		t.Fatalf("updated history has %d messages, want 2", len(updated))
	}
	if updated[1].Role != provider.RoleAssistant || updated[1].Content != "hello there" {
		t.Errorf("updated[1] = %+v, want assistant message with content %q", updated[1], "hello there")
	}
}

func TestRun_ExecutesToolCallAndContinuesUntilNoToolCalls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "greeting.txt")
	args, err := json.Marshal(map[string]string{"path": path, "content": "hi"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{
				{ID: "call-1", Name: "write", Arguments: args},
			}},
			{Text: "done"},
		},
	}
	history := []provider.Message{
		{Role: provider.RoleUser, Content: "write a greeting file"},
	}

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if text != "done" {
		t.Errorf("text = %q, want %q", text, "done")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("file content = %q, want %q", got, "hi")
	}

	// user, assistant(tool call), tool-result, assistant(final)
	if len(updated) != 4 {
		t.Fatalf("updated history has %d messages, want 4: %+v", len(updated), updated)
	}
	assistantCall := updated[1]
	if assistantCall.Role != provider.RoleAssistant || len(assistantCall.ToolCalls) != 1 {
		t.Errorf("updated[1] = %+v, want assistant message carrying the tool call", assistantCall)
	}
	toolResult := updated[2]
	if toolResult.Role != provider.RoleTool || toolResult.ToolCallID != "call-1" {
		t.Errorf("updated[2] = %+v, want tool-result message for call-1", toolResult)
	}
	wantResult := fmt.Sprintf("wrote %d bytes to %s", len("hi"), path)
	if toolResult.Content != wantResult {
		t.Errorf("toolResult.Content = %q, want %q", toolResult.Content, wantResult)
	}
	final := updated[3]
	if final.Role != provider.RoleAssistant || final.Content != "done" {
		t.Errorf("updated[3] = %+v, want final assistant message", final)
	}
}

func TestRun_CombinesTextAndToolCallsIntoOneMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	args, err := json.Marshal(map[string]string{"path": path, "content": "hi"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{
				Text:      "sure, writing that now",
				ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "write", Arguments: args}},
			},
			{Text: "done"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "write a file"}}

	if _, updated, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	} else {
		msg := updated[1]
		if msg.Role != provider.RoleAssistant {
			t.Fatalf("updated[1].Role = %v, want assistant", msg.Role)
		}
		if msg.Content != "sure, writing that now" {
			t.Errorf("updated[1].Content = %q, want the response text", msg.Content)
		}
		if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call-1" {
			t.Errorf("updated[1].ToolCalls = %+v, want the single tool call in the same message", msg.ToolCalls)
		}
	}
}

func TestRun_ExecutesToolCallsSequentiallyInOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	writeArgs, err := json.Marshal(map[string]string{"path": path, "content": "v1"})
	if err != nil {
		t.Fatalf("marshal write args: %v", err)
	}
	// This edit only succeeds if the preceding write already ran, proving
	// the two tool calls in this single response executed in order.
	editArgs, err := json.Marshal(map[string]string{"path": path, "old_text": "v1", "new_text": "v2"})
	if err != nil {
		t.Fatalf("marshal edit args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{
				{ID: "call-1", Name: "write", Arguments: writeArgs},
				{ID: "call-2", Name: "edit", Arguments: editArgs},
			}},
			{Text: "done"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "write then edit"}}

	if _, updated, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	} else {
		writeResult := updated[2]
		editResult := updated[3]
		if writeResult.ToolCallID != "call-1" || editResult.ToolCallID != "call-2" {
			t.Fatalf("tool-result order = %q, %q, want call-1 then call-2", writeResult.ToolCallID, editResult.ToolCallID)
		}
		if editResult.Content != fmt.Sprintf("edited %s", path) {
			t.Errorf("edit result = %q, want success (proves it ran after the write)", editResult.Content)
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("file content = %q, want %q", got, "v2")
	}
}

func TestRun_FailingToolCallFeedsErrorBackAndContinues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")
	readArgs, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("marshal read args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "read", Arguments: readArgs}}},
			{Text: "looks like that file doesn't exist"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "read a missing file"}}

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{})
	if err != nil {
		t.Fatalf("Run should not abort on a failing tool call, got error: %v", err)
	}
	if text != "looks like that file doesn't exist" {
		t.Errorf("text = %q, want the model's follow-up after seeing the tool error", text)
	}

	toolResult := updated[2]
	if toolResult.Role != provider.RoleTool || toolResult.ToolCallID != "call-1" {
		t.Fatalf("updated[2] = %+v, want tool-result message for call-1", toolResult)
	}
	if toolResult.Content == "" {
		t.Error("expected a non-empty error result fed back to the model")
	}
}

func TestRun_MultipleRoundsOfToolCallsBeforeTerminating(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.txt")
	pathB := filepath.Join(dir, "b.txt")

	writeA, err := json.Marshal(map[string]string{"path": pathA, "content": "A"})
	if err != nil {
		t.Fatalf("marshal writeA args: %v", err)
	}
	writeB, err := json.Marshal(map[string]string{"path": pathB, "content": "B"})
	if err != nil {
		t.Fatalf("marshal writeB args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "write", Arguments: writeA}}},
			{ToolCalls: []provider.ToolCall{{ID: "call-2", Name: "write", Arguments: writeB}}},
			{Text: "wrote both files"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "write two files"}}

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if text != "wrote both files" {
		t.Errorf("text = %q, want %q", text, "wrote both files")
	}
	if p.calls != 3 {
		t.Errorf("provider was called %d times, want 3 (two tool rounds + final)", p.calls)
	}
	// user, assistant/tool x2 rounds, final assistant = 6 messages.
	if len(updated) != 6 {
		t.Fatalf("updated history has %d messages, want 6: %+v", len(updated), updated)
	}
	for _, path := range []string{pathA, pathB} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to have been written: %v", path, err)
		}
	}
}

func TestRun_UnknownToolNameReturnsError(t *testing.T) {
	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{
				{ID: "call-1", Name: "does-not-exist", Arguments: json.RawMessage(`{}`)},
			}},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "do something"}}

	if _, _, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{}); err == nil {
		t.Fatal("expected an error when the Provider requests a tool name outside the dispatch table, per tool.Call's contract that this is a bug, not a soft failure")
	}
}

func TestRun_OnToolCallFiresBeforeExecutionAndOnToolResultAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "greeting.txt")
	args, err := json.Marshal(map[string]string{"path": path, "content": "hi"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "write", Arguments: args}}},
			{Text: "done"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "write a greeting file"}}

	type toolCallEvent struct {
		name string
		args json.RawMessage
	}
	type toolResultEvent struct {
		name   string
		result string
		err    error
	}
	var calls []toolCallEvent
	var results []toolResultEvent

	cb := agent.Callbacks{
		OnToolCall: func(name string, args json.RawMessage) {
			calls = append(calls, toolCallEvent{name, args})
		},
		OnToolResult: func(name, result string, err error) {
			results = append(results, toolResultEvent{name, result, err})
		},
	}

	if _, _, err := agent.Run(context.Background(), p, tool.Tools, history, cb); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("OnToolCall fired %d times, want 1: %+v", len(calls), calls)
	}
	if calls[0].name != "write" || string(calls[0].args) != string(args) {
		t.Errorf("OnToolCall event = %+v, want name %q args %s", calls[0], "write", args)
	}

	if len(results) != 1 {
		t.Fatalf("OnToolResult fired %d times, want 1: %+v", len(results), results)
	}
	wantResult := fmt.Sprintf("wrote %d bytes to %s", len("hi"), path)
	if results[0].name != "write" || results[0].result != wantResult || results[0].err != nil {
		t.Errorf("OnToolResult event = %+v, want name %q result %q err nil", results[0], "write", wantResult)
	}
}

func TestRun_OnToolResultReceivesSameStringFedBackIntoHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")
	readArgs, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("marshal read args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "read", Arguments: readArgs}}},
			{Text: "that file doesn't exist"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "read a missing file"}}

	var gotResult string
	var gotErr error
	cb := agent.Callbacks{
		OnToolResult: func(name, result string, err error) {
			gotResult = result
			gotErr = err
		},
	}

	_, updated, err := agent.Run(context.Background(), p, tool.Tools, history, cb)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	toolResult := updated[2]
	if gotResult != toolResult.Content {
		t.Errorf("OnToolResult result = %q, want it to match the string fed into history %q", gotResult, toolResult.Content)
	}
	if gotErr == nil {
		t.Error("OnToolResult err = nil, want the underlying tool error for a failing call")
	}
}

func TestRun_OnTextFiresForIntermediateAndFinalText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	args, err := json.Marshal(map[string]string{"path": path, "content": "hi"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{
				Text:      "sure, writing that now",
				ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "write", Arguments: args}},
			},
			{Text: "done"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "write a file"}}

	var texts []string
	cb := agent.Callbacks{
		OnText: func(text string) {
			texts = append(texts, text)
		},
	}

	if _, _, err := agent.Run(context.Background(), p, tool.Tools, history, cb); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := []string{"sure, writing that now", "done"}
	if len(texts) != len(want) {
		t.Fatalf("OnText fired %d times, want %d: %+v", len(texts), len(want), texts)
	}
	for i, w := range want {
		if texts[i] != w {
			t.Errorf("texts[%d] = %q, want %q", i, texts[i], w)
		}
	}
}

func TestRun_NilCallbackFieldsAreNotInvoked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	args, err := json.Marshal(map[string]string{"path": path, "content": "hi"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "write", Arguments: args}}},
			{Text: "done"},
		},
	}
	history := []provider.Message{{Role: provider.RoleUser, Content: "write a file"}}

	// A zero-value Callbacks{} must not panic despite every field being nil.
	if _, _, err := agent.Run(context.Background(), p, tool.Tools, history, agent.Callbacks{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRun_DoesNotAliasCallersHistoryBackingArray(t *testing.T) {
	backing := make([]provider.Message, 1, 4)
	backing[0] = provider.Message{Role: provider.RoleUser, Content: "hi"}

	p := &fakeProvider{responses: []provider.Response{{Text: "hello"}}}
	if _, _, err := agent.Run(context.Background(), p, tool.Tools, backing, agent.Callbacks{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Grow the original slice back over its spare capacity and check Run
	// didn't write the appended message into it behind the caller's back.
	grown := backing[:cap(backing)]
	if grown[1].Role != "" || grown[1].Content != "" {
		t.Errorf("Run wrote into the caller's spare backing-array capacity: %+v", grown[1])
	}
}

func TestRun_AlreadyCancelledContext_ReturnsImmediatelyWithoutCallingProvider(t *testing.T) {
	p := &fakeProvider{responses: []provider.Response{{Text: "should not be reached"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	history := []provider.Message{{Role: provider.RoleUser, Content: "hi"}}

	text, updated, err := agent.Run(ctx, p, tool.Tools, history, agent.Callbacks{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(updated) != 1 || updated[0].Content != "hi" {
		t.Errorf("updated = %+v, want unchanged original history", updated)
	}
	if p.calls != 0 {
		t.Errorf("provider was called %d times, want 0: an already-cancelled context should short-circuit before calling Complete", p.calls)
	}
}

func TestRun_CancelledDuringProviderComplete_ReturnsPromptlyWithHistoryUnchanged(t *testing.T) {
	started := make(chan struct{})
	p := &blockingProvider{started: started}
	ctx, cancel := context.WithCancel(context.Background())
	history := []provider.Message{{Role: provider.RoleUser, Content: "hi"}}

	var text string
	var updated []provider.Message
	var err error
	done := make(chan struct{})
	go func() {
		text, updated, err = agent.Run(ctx, p, tool.Tools, history, agent.Callbacks{})
		close(done)
	}()

	<-started
	cancel()
	waitDone(t, done)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(updated) != 1 || updated[0].Content != "hi" {
		t.Errorf("updated = %+v, want unchanged original history (no message appended for the abandoned Complete call)", updated)
	}
}

func TestRun_CancelledDuringToolCall_ReturnsPromptlyWithoutSyntheticResult(t *testing.T) {
	started := make(chan struct{})
	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "block", Arguments: json.RawMessage(`{}`)}}},
		},
	}
	tools := map[string]tool.Tool{"block": blockingTool(started)}
	ctx, cancel := context.WithCancel(context.Background())
	history := []provider.Message{{Role: provider.RoleUser, Content: "run the blocking tool"}}

	var toolResultFired bool
	cb := agent.Callbacks{
		OnToolResult: func(name, result string, err error) {
			toolResultFired = true
		},
	}

	var updated []provider.Message
	var err error
	done := make(chan struct{})
	go func() {
		_, updated, err = agent.Run(ctx, p, tools, history, cb)
		close(done)
	}()

	<-started
	cancel()
	waitDone(t, done)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	// user, assistant(call-1) — no tool-result for the abandoned call-1.
	if len(updated) != 2 {
		t.Fatalf("updated history has %d messages, want 2: %+v", len(updated), updated)
	}
	if updated[1].Role != provider.RoleAssistant || len(updated[1].ToolCalls) != 1 {
		t.Errorf("updated[1] = %+v, want assistant message carrying the tool call", updated[1])
	}
	if toolResultFired {
		t.Error("OnToolResult fired for a tool call abandoned mid-flight due to cancellation")
	}
}

func TestRun_CancelledDuringToolCall_PreservesEarlierRoundToolResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeArgs, err := json.Marshal(map[string]string{"path": path, "content": "A"})
	if err != nil {
		t.Fatalf("marshal write args: %v", err)
	}

	started := make(chan struct{})
	p := &fakeProvider{
		responses: []provider.Response{
			{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "write", Arguments: writeArgs}}},
			{ToolCalls: []provider.ToolCall{{ID: "call-2", Name: "block", Arguments: json.RawMessage(`{}`)}}},
		},
	}
	tools := map[string]tool.Tool{
		"write": tool.Tools["write"],
		"block": blockingTool(started),
	}
	ctx, cancel := context.WithCancel(context.Background())
	history := []provider.Message{{Role: provider.RoleUser, Content: "write then block"}}

	var updated []provider.Message
	var runErr error
	done := make(chan struct{})
	go func() {
		_, updated, runErr = agent.Run(ctx, p, tools, history, agent.Callbacks{})
		close(done)
	}()

	<-started
	cancel()
	waitDone(t, done)

	if !errors.Is(runErr, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", runErr)
	}

	// user, assistant(call-1), tool-result(call-1), assistant(call-2) — no
	// tool-result for call-2, abandoned mid-flight.
	if len(updated) != 4 {
		t.Fatalf("updated history has %d messages, want 4: %+v", len(updated), updated)
	}
	toolResult := updated[2]
	if toolResult.Role != provider.RoleTool || toolResult.ToolCallID != "call-1" {
		t.Errorf("updated[2] = %+v, want preserved tool-result for call-1 from the earlier round", toolResult)
	}
	wantResult := fmt.Sprintf("wrote %d bytes to %s", len("A"), path)
	if toolResult.Content != wantResult {
		t.Errorf("toolResult.Content = %q, want %q", toolResult.Content, wantResult)
	}
	for _, m := range updated {
		if m.ToolCallID == "call-2" {
			t.Errorf("expected no tool-result message for call-2 (cancelled mid-flight), got %+v", m)
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(got) != "A" {
		t.Errorf("file content = %q, want %q (proves call-1 actually ran before cancellation)", got, "A")
	}
}
