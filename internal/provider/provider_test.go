package provider

import (
	"errors"
	"testing"
)

func TestEventTypeSwitch(t *testing.T) {
	events := []Event{
		TextDeltaEvent{Text: "hi"},
		ToolCallEvent{ID: "call_1", Name: "read_file", ArgsJSON: `{"path":"a"}`},
		DoneEvent{FinishReason: "stop", ModelUsed: "openai/gpt-4o", Usage: Usage{InputTokens: 1}},
	}

	var texts, calls, dones int
	for _, ev := range events {
		switch e := ev.(type) {
		case TextDeltaEvent:
			texts++
			if e.Text != "hi" {
				t.Errorf("TextDeltaEvent.Text = %q, want %q", e.Text, "hi")
			}
		case ToolCallEvent:
			calls++
			if e.ID != "call_1" || e.Name != "read_file" {
				t.Errorf("unexpected ToolCallEvent: %+v", e)
			}
		case DoneEvent:
			dones++
			if e.FinishReason != "stop" {
				t.Errorf("DoneEvent.FinishReason = %q, want %q", e.FinishReason, "stop")
			}
		default:
			t.Fatalf("unhandled event type %T", ev)
		}
	}
	if texts != 1 || calls != 1 || dones != 1 {
		t.Fatalf("got texts=%d calls=%d dones=%d, want 1 each", texts, calls, dones)
	}
}

func TestRequestConstruction(t *testing.T) {
	req := Request{
		Model:        "openrouter/auto",
		SystemPrompt: "be terse",
		Messages: []Message{
			{Role: "user", Content: "hi"},
			{Role: "tool", Content: "result", ToolCallID: "call_1"},
		},
		Tools: []ToolDef{{Name: "read_file", Description: "Read a file"}},
	}

	if len(req.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(req.Messages))
	}
	if req.Messages[1].ToolCallID != "call_1" {
		t.Errorf("Messages[1].ToolCallID = %q, want %q", req.Messages[1].ToolCallID, "call_1")
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "read_file" {
		t.Errorf("Tools = %+v, want one ToolDef named read_file", req.Tools)
	}
}

func TestProviderErrorMessageAndUnwrap(t *testing.T) {
	cause := errors.New("boom")
	err := &ProviderError{Kind: ErrorKindRateLimited, Cause: cause}

	if got, want := err.Error(), "provider error (rate_limited): boom"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false, want true")
	}
}

func TestProviderErrorMessageWithoutCause(t *testing.T) {
	err := &ProviderError{Kind: ErrorKindUnknown}
	if got, want := err.Error(), "provider error (unknown)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
