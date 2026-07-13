package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgoodness/liam/provider"
	"github.com/mgoodness/liam/skill"
	"github.com/mgoodness/liam/tool"
)

// fakeProvider returns a scripted sequence of Responses, one per call to
// Complete, regardless of the messages passed in. lastMessages records the
// history from the most recent call, so a test can inspect exactly what a
// turn sent to the model.
type fakeProvider struct {
	responses    []provider.Response
	errs         []error
	calls        int
	lastMessages []provider.Message
}

func (f *fakeProvider) Complete(ctx context.Context, messages []provider.Message) (provider.Response, error) {
	f.lastMessages = messages

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
	in := strings.NewReader("hi there\r/exit\r")
	var out bytes.Buffer
	p := &fakeProvider{responses: []provider.Response{{Text: "hello back"}}}

	runSession(context.Background(), in, &out, &out, p, tool.Tools, "you are liam", nil)

	if !strings.Contains(out.String(), "hello back") {
		t.Errorf("output = %q, want it to contain the final assistant text", out.String())
	}
}

func TestRunSession_MidTurnErrorIsPrintedAndSessionContinues(t *testing.T) {
	in := strings.NewReader("first message\rsecond message\r/exit\r")
	var out, errOut bytes.Buffer
	p := &fakeProvider{
		errs:      []error{errors.New("network failure")},
		responses: []provider.Response{{}, {Text: "recovered"}},
	}

	runSession(context.Background(), in, &out, &errOut, p, tool.Tools, "you are liam", nil)

	if !strings.Contains(errOut.String(), "network failure") {
		t.Errorf("errOut = %q, want it to contain the mid-turn error", errOut.String())
	}
	if !strings.Contains(out.String(), "recovered") {
		t.Errorf("out = %q, want the session to have continued to the next prompt and completed it", out.String())
	}
}

func TestRunSession_PrintsProgressForIntermediateTextAndToolCalls(t *testing.T) {
	in := strings.NewReader("do it\r/exit\r")
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

	runSession(context.Background(), in, &out, &out, p, tool.Tools, "you are liam", nil)
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

func TestRunSession_ExplicitSkillInvocationLoadsBodyAndPassesTrailingText(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "skill body instructions")
	skills := []skill.Skill{{Name: "pdf-processing", Path: dir}}

	in := strings.NewReader("/pdf-processing summarize report.pdf\r/exit\r")
	var out bytes.Buffer
	p := &fakeProvider{responses: []provider.Response{{Text: "done"}}}

	runSession(context.Background(), in, &out, &out, p, tool.Tools, "you are liam", skills)

	if len(p.lastMessages) == 0 {
		t.Fatal("no messages reached the provider")
	}
	sent := p.lastMessages[len(p.lastMessages)-1]
	if sent.Role != provider.RoleUser {
		t.Fatalf("last message role = %v, want RoleUser", sent.Role)
	}
	if !strings.Contains(sent.Content, "skill body instructions") {
		t.Errorf("sent content = %q, want it to contain the skill's SKILL.md body", sent.Content)
	}
	if !strings.Contains(sent.Content, "summarize report.pdf") {
		t.Errorf("sent content = %q, want it to contain the trailing text unchanged", sent.Content)
	}
}

func TestRunSession_DisableModelInvocationSkillIsExplicitlyInvocable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "internal-only body")
	skills := []skill.Skill{{Name: "internal-only", Path: dir, DisableModelInvocation: true}}

	in := strings.NewReader("/internal-only\r/exit\r")
	var out bytes.Buffer
	p := &fakeProvider{responses: []provider.Response{{Text: "done"}}}

	runSession(context.Background(), in, &out, &out, p, tool.Tools, "you are liam", skills)

	if len(p.lastMessages) == 0 {
		t.Fatal("no messages reached the provider")
	}
	sent := p.lastMessages[len(p.lastMessages)-1]
	if !strings.Contains(sent.Content, "internal-only body") {
		t.Errorf("sent content = %q, want the disable-model-invocation skill's body loaded anyway", sent.Content)
	}
}

func TestRunSession_UnmatchedSlashNameIsSentThroughAsOrdinaryMessage(t *testing.T) {
	in := strings.NewReader("/no-such-skill\r/exit\r")
	var out bytes.Buffer
	p := &fakeProvider{responses: []provider.Response{{Text: "done"}}}

	runSession(context.Background(), in, &out, &out, p, tool.Tools, "you are liam", nil)

	if len(p.lastMessages) == 0 {
		t.Fatal("no messages reached the provider: an unmatched /name must not crash or stall the session")
	}
	sent := p.lastMessages[len(p.lastMessages)-1]
	if sent.Content != "/no-such-skill" {
		t.Errorf("sent content = %q, want the literal unmatched text passed through unchanged", sent.Content)
	}
}
