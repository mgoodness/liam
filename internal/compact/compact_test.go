package compact

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/provider"
)

// fakeProvider scripts one []provider.Event (or error) per call to Stream,
// in call order, and records every Request it was called with.
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

func userMsg(text string) provider.Message { return provider.Message{Role: "user", Content: text} }

func TestCompactReturnsUnchangedWhenWithinWindow(t *testing.T) {
	fp := &fakeProvider{}
	messages := []provider.Message{userMsg("1"), {Role: "assistant", Content: "a1"}, userMsg("2")}

	got, compacted, err := Compact(context.Background(), fp, "model", messages, 5)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if compacted {
		t.Error("compacted = true, want false (fewer turns than the window)")
	}
	if len(fp.requests) != 0 {
		t.Errorf("Provider.Stream called %d times, want 0", len(fp.requests))
	}
	if len(got) != len(messages) {
		t.Fatalf("len(got) = %d, want %d (unmodified)", len(got), len(messages))
	}
}

func TestCompactKeepsSlidingWindowVerbatimAndSummarizesOlder(t *testing.T) {
	var messages []provider.Message
	for i := 1; i <= 5; i++ {
		messages = append(messages,
			userMsg("turn"),
			provider.Message{Role: "assistant", Content: "reply"},
		)
	}
	// 5 user turns total; keep the most recent 2 verbatim.
	fp := &fakeProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "the summary"}, provider.DoneEvent{}},
	}}

	got, compacted, err := Compact(context.Background(), fp, "model", messages, 2)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if !compacted {
		t.Fatal("compacted = false, want true")
	}
	if len(fp.requests) != 1 {
		t.Fatalf("Provider.Stream called %d times, want 1", len(fp.requests))
	}

	// Summarizer must never see the 2 most recent turns.
	summarizerMessages := fp.requests[0].Messages
	if len(summarizerMessages) != 6+1 { // 3 old turns * 2 messages each, plus the trailing instruction
		t.Fatalf("summarizer saw %d messages, want 7: %+v", len(summarizerMessages), summarizerMessages)
	}

	// Result: summary + the 2 most recent turns verbatim.
	if len(got) != 1+4 {
		t.Fatalf("len(got) = %d, want 5: %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != summaryPrefix+"the summary" {
		t.Errorf("got[0] = %+v, want the summary message", got[0])
	}
	for i := 1; i < len(got); i++ {
		if want := messages[len(messages)-4+i-1]; !reflect.DeepEqual(got[i], want) {
			t.Errorf("got[%d] = %+v, want the verbatim recent message %+v", i, got[i], want)
		}
	}
}

func TestCompactSkillContentExemptFromSummarizerAndReappendedVerbatim(t *testing.T) {
	skillBody := "# my-skill\n\nDo the thing this way."
	messages := []provider.Message{
		userMsg("please use my-skill"),
		{
			Role:      "assistant",
			ToolCalls: []provider.ToolCall{{ID: "call_1", Name: "activate_skill", ArgsJSON: `{"name":"my-skill"}`}},
		},
		{Role: "tool", Content: skillBody, ToolCallID: "call_1"},
		{Role: "assistant", Content: "got it, using my-skill"},
		userMsg("turn 2"), {Role: "assistant", Content: "reply 2"},
		userMsg("turn 3"), {Role: "assistant", Content: "reply 3"},
		userMsg("turn 4 (recent)"), {Role: "assistant", Content: "reply 4"},
	}
	fp := &fakeProvider{turns: [][]provider.Event{
		{provider.TextDeltaEvent{Text: "summary"}},
	}}

	got, compacted, err := Compact(context.Background(), fp, "model", messages, 1)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if !compacted {
		t.Fatal("compacted = false, want true")
	}

	for _, m := range fp.requests[0].Messages {
		if strings.Contains(m.Content, skillBody) {
			t.Errorf("summarizer input contained the skill body: %+v", m)
		}
	}

	var sawAssistantCall, sawToolBody bool
	for _, m := range got {
		if m.Role == "assistant" && len(m.ToolCalls) == 1 && m.ToolCalls[0].Name == "activate_skill" {
			sawAssistantCall = true
		}
		if m.Role == "tool" && m.Content == skillBody && m.ToolCallID == "call_1" {
			sawToolBody = true
		}
	}
	if !sawAssistantCall {
		t.Errorf("result missing the re-appended activate_skill call: %+v", got)
	}
	if !sawToolBody {
		t.Errorf("result missing the re-appended skill body verbatim: %+v", got)
	}
}

func TestCompactPropagatesSummarizerError(t *testing.T) {
	wantErr := errors.New("boom")
	messages := []provider.Message{
		userMsg("1"), {Role: "assistant", Content: "a1"},
		userMsg("2"), {Role: "assistant", Content: "a2"},
		userMsg("3"), {Role: "assistant", Content: "a3"},
	}
	fp := &fakeProvider{errs: []error{wantErr}}

	_, compacted, err := Compact(context.Background(), fp, "model", messages, 1)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Compact() error = %v, want %v", err, wantErr)
	}
	if compacted {
		t.Error("compacted = true, want false on summarizer failure")
	}
}
