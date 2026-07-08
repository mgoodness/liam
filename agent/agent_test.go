package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

func TestRun_NoToolCalls_ReturnsFinalText(t *testing.T) {
	p := &fakeProvider{
		responses: []provider.Response{
			{Text: "hello there"},
		},
	}
	history := []provider.Message{
		{Role: provider.RoleUser, Content: "hi"},
	}

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history)
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

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history)
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

	if _, updated, err := agent.Run(context.Background(), p, tool.Tools, history); err != nil {
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

	if _, updated, err := agent.Run(context.Background(), p, tool.Tools, history); err != nil {
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

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history)
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

	text, updated, err := agent.Run(context.Background(), p, tool.Tools, history)
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

	if _, _, err := agent.Run(context.Background(), p, tool.Tools, history); err == nil {
		t.Fatal("expected an error when the Provider requests a tool name outside the dispatch table, per tool.Call's contract that this is a bug, not a soft failure")
	}
}

func TestRun_DoesNotAliasCallersHistoryBackingArray(t *testing.T) {
	backing := make([]provider.Message, 1, 4)
	backing[0] = provider.Message{Role: provider.RoleUser, Content: "hi"}

	p := &fakeProvider{responses: []provider.Response{{Text: "hello"}}}
	if _, _, err := agent.Run(context.Background(), p, tool.Tools, backing); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Grow the original slice back over its spare capacity and check Run
	// didn't write the appended message into it behind the caller's back.
	grown := backing[:cap(backing)]
	if grown[1].Role != "" || grown[1].Content != "" {
		t.Errorf("Run wrote into the caller's spare backing-array capacity: %+v", grown[1])
	}
}
