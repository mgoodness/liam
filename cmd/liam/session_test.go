package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/tool"
)

// fakeProvider returns a scripted sequence of Responses, one per call to
// Complete, regardless of the messages passed in.
type fakeProvider struct {
	responses []provider.Response
	errs      []error
	calls     int
}

func (f *fakeProvider) Complete(ctx context.Context, messages []provider.Message) (provider.Response, error) {
	i := f.calls
	f.calls++
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	if err != nil {
		return provider.Response{}, err
	}
	return f.responses[i], nil
}

func TestRunSession_SubmittedMessageDrivesAgentLoopAndPrintsResult(t *testing.T) {
	in := strings.NewReader("hi there\n\n/exit\n")
	var out bytes.Buffer
	p := &fakeProvider{responses: []provider.Response{{Text: "hello back"}}}

	runSession(context.Background(), in, &out, &out, p, tool.Tools, "you are liam")

	if !strings.Contains(out.String(), "hello back") {
		t.Errorf("output = %q, want it to contain the final assistant text", out.String())
	}
}

func TestRunSession_MidTurnErrorIsPrintedAndSessionContinues(t *testing.T) {
	in := strings.NewReader("first message\n\nsecond message\n\n/exit\n")
	var out, errOut bytes.Buffer
	p := &fakeProvider{
		errs:      []error{errors.New("network failure")},
		responses: []provider.Response{{}, {Text: "recovered"}},
	}

	runSession(context.Background(), in, &out, &errOut, p, tool.Tools, "you are liam")

	if !strings.Contains(errOut.String(), "network failure") {
		t.Errorf("errOut = %q, want it to contain the mid-turn error", errOut.String())
	}
	if !strings.Contains(out.String(), "recovered") {
		t.Errorf("out = %q, want the session to have continued to the next prompt and completed it", out.String())
	}
}

func TestRunSession_PrintsProgressForIntermediateTextAndToolCalls(t *testing.T) {
	in := strings.NewReader("do it\n\n/exit\n")
	var out bytes.Buffer

	args, err := json.Marshal(map[string]string{"command": "echo hi"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	p := &fakeProvider{
		responses: []provider.Response{
			{
				Text:      "working on it",
				ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "bash", Arguments: args}},
			},
			{Text: "all done"},
		},
	}

	runSession(context.Background(), in, &out, &out, p, "you are liam")
	got := out.String()

	if !strings.Contains(got, "working on it") {
		t.Errorf("out = %q, want the intermediate assistant text printed", got)
	}
	if !strings.Contains(got, "bash") || !strings.Contains(got, "echo hi") {
		t.Errorf("out = %q, want a tool-call summary naming the tool and its command", got)
	}
	if !strings.Contains(got, "all done") {
		t.Errorf("out = %q, want the final assistant text printed", got)
	}
}
