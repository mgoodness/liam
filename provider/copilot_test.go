package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/mgoodness/liam/tool"
)

// newTestCopilot returns a Copilot whose Authenticator already has a
// valid, unexpired credential on disk, so Complete never has to touch the
// device-flow endpoints — and whose Endpoint points at endpoint (normally
// an httptest.Server standing in for api.githubcopilot.com).
func newTestCopilot(t *testing.T, endpoint, copilotToken, model string, tools ...tool.Definition) *Copilot {
	t.Helper()
	a := NewAuthenticator()
	a.CredentialsPath = filepath.Join(t.TempDir(), "credentials.json")
	if err := saveCredentials(a.CredentialsPath, &Credentials{
		GitHubToken:        "gho_test",
		CopilotToken:       copilotToken,
		CopilotTokenExpiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}

	c := NewCopilot(a, model, tools)
	c.Endpoint = endpoint
	return c
}

func TestCopilotComplete_TextResponse(t *testing.T) {
	var gotAuth, gotIntegrationID, gotEditorVersion string
	var gotBody chatRequest

	srv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotIntegrationID = r.Header.Get("Copilot-Integration-Id")
		gotEditorVersion = r.Header.Get("Editor-Version")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		writeJSON(t, w, chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "hello there"}},
			},
		})
	})

	c := newTestCopilot(t, srv.URL, "tid=test123", "")

	got, err := c.Complete(context.Background(), []Message{
		{Role: RoleSystem, Content: "you are liam"},
		{Role: RoleUser, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotAuth != "Bearer tid=test123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer tid=test123")
	}
	if gotIntegrationID != copilotIntegrationID {
		t.Errorf("Copilot-Integration-Id header = %q, want %q", gotIntegrationID, copilotIntegrationID)
	}
	if gotEditorVersion == "" {
		t.Error("Editor-Version header is empty, want a spoofed IDE version (api.githubcopilot.com rejects requests without one)")
	}
	if gotBody.Model != defaultModel {
		t.Errorf("request model = %q, want default %q", gotBody.Model, defaultModel)
	}
	if len(gotBody.Messages) != 2 || gotBody.Messages[0].Role != "system" || gotBody.Messages[1].Role != "user" {
		t.Errorf("request messages = %+v, want system+user turns", gotBody.Messages)
	}

	if got.Text != "hello there" {
		t.Errorf("Complete().Text = %q, want %q", got.Text, "hello there")
	}
	if len(got.ToolCalls) != 0 {
		t.Errorf("Complete().ToolCalls = %+v, want none", got.ToolCalls)
	}
}

func TestCopilotComplete_ModelOverride(t *testing.T) {
	var gotBody chatRequest

	srv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		writeJSON(t, w, chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: "ok"}}},
		})
	})

	c := newTestCopilot(t, srv.URL, "tid=test123", "gpt-5-mini")

	if _, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotBody.Model != "gpt-5-mini" {
		t.Errorf("request model = %q, want override %q", gotBody.Model, "gpt-5-mini")
	}
}

func TestCopilotComplete_ToolCallResponse(t *testing.T) {
	srv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessage{
						Role: "assistant",
						ToolCalls: []chatToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: chatFunctionCall{
									Name:      "read",
									Arguments: `{"path":"main.go"}`,
								},
							},
							{
								ID:   "call_2",
								Type: "function",
								Function: chatFunctionCall{
									Name:      "bash",
									Arguments: `{"command":"go build ./..."}`,
								},
							},
						},
					},
				},
			},
		})
	})

	c := newTestCopilot(t, srv.URL, "tid=test123", "")

	got, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "build it"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if got.Text != "" {
		t.Errorf("Complete().Text = %q, want empty", got.Text)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("Complete().ToolCalls has %d entries, want 2", len(got.ToolCalls))
	}

	want := []ToolCall{
		{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
		{ID: "call_2", Name: "bash", Arguments: json.RawMessage(`{"command":"go build ./..."}`)},
	}
	for i, w := range want {
		g := got.ToolCalls[i]
		if g.ID != w.ID || g.Name != w.Name || string(g.Arguments) != string(w.Arguments) {
			t.Errorf("ToolCalls[%d] = %+v, want %+v", i, g, w)
		}
	}
}

func TestCopilotComplete_SendsToolDefinitions(t *testing.T) {
	readDef := tool.Tools["read"].Definition
	bashDef := tool.Tools["bash"].Definition

	var gotBody chatRequest

	srv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		writeJSON(t, w, chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: "ok"}}},
		})
	})

	c := newTestCopilot(t, srv.URL, "tid=test123", "", readDef, bashDef)

	if _, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(gotBody.Tools) != 2 {
		t.Fatalf("request tools has %d entries, want 2", len(gotBody.Tools))
	}
	if gotBody.Tools[0].Type != "function" {
		t.Errorf("Tools[0].Type = %q, want %q", gotBody.Tools[0].Type, "function")
	}
	if gotBody.Tools[0].Function.Name != "read" {
		t.Errorf("Tools[0].Function.Name = %q, want %q", gotBody.Tools[0].Function.Name, "read")
	}
	if gotBody.Tools[1].Function.Name != "bash" {
		t.Errorf("Tools[1].Function.Name = %q, want %q", gotBody.Tools[1].Function.Name, "bash")
	}
	if gotBody.Tools[0].Function.Parameters.Type != readDef.Parameters.Type {
		t.Errorf("Tools[0].Function.Parameters = %+v, want %+v", gotBody.Tools[0].Function.Parameters, readDef.Parameters)
	}
}

func TestCopilotComplete_NonAuthErrorNotRetried(t *testing.T) {
	calls := 0
	srv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	})

	c := newTestCopilot(t, srv.URL, "tid=test123", "")

	_, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err == nil {
		t.Fatal("Complete: want error, got nil")
	}
	if calls != 1 {
		t.Errorf("server received %d requests, want exactly 1 (no internal retry)", calls)
	}
}

// TestCopilotComplete_AuthErrorRetriesOnce covers the case where the
// stored Copilot token looks unexpired but the API rejects it anyway
// (e.g. revoked out-of-band): Complete should force a full device-flow
// re-login via Authenticator.Login and retry the chat request exactly
// once with the freshly issued token.
func TestCopilotComplete_AuthErrorRetriesOnce(t *testing.T) {
	tokens := []string{"tid=stale", "tid=fresh"}
	callIndex := 0

	chatSrv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth := r.Header.Get("Authorization")
		wantAuth := "Bearer " + tokens[callIndex]
		callIndex++
		if gotAuth != wantAuth {
			t.Errorf("call %d Authorization = %q, want %q", callIndex, gotAuth, wantAuth)
		}
		if gotAuth == "Bearer tid=stale" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(t, w, chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: "recovered"}}},
		})
	})

	deviceCodeSrv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, deviceCodeResponse{
			DeviceCode:      "device-1",
			UserCode:        "CODE",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        0,
		})
	})
	accessTokenSrv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, accessTokenResponse{AccessToken: "gho_new"})
	})
	copilotTokenSrv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, copilotTokenResponse{Token: "tid=fresh", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	})

	a := NewAuthenticator()
	a.CredentialsPath = filepath.Join(t.TempDir(), "credentials.json")
	a.Endpoints = Endpoints{
		DeviceCode:   deviceCodeSrv.URL,
		AccessToken:  accessTokenSrv.URL,
		CopilotToken: copilotTokenSrv.URL,
	}
	a.Prompt = func(string, string) {}
	if err := saveCredentials(a.CredentialsPath, &Credentials{
		GitHubToken:        "gho_old",
		CopilotToken:       "tid=stale",
		CopilotTokenExpiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}

	c := NewCopilot(a, "", nil)
	c.Endpoint = chatSrv.URL

	got, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Text != "recovered" {
		t.Errorf("Complete().Text = %q, want %q", got.Text, "recovered")
	}
	if callIndex != 2 {
		t.Errorf("chat endpoint received %d requests, want exactly 2 (initial + one retry)", callIndex)
	}
}

// TestCopilotComplete_RoundTripsToolCallHistory covers the agent-loop
// case: a prior assistant turn that requested a tool call, followed by
// that tool's result, must both survive into the outgoing request with
// their tool_calls/tool_call_id intact — otherwise the API can't
// correlate the tool result with the call it answers on a second turn.
func TestCopilotComplete_RoundTripsToolCallHistory(t *testing.T) {
	var gotBody chatRequest

	srv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		writeJSON(t, w, chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: "done"}}},
		})
	})

	c := newTestCopilot(t, srv.URL, "tid=test123", "")

	history := []Message{
		{Role: RoleUser, Content: "read main.go"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
			},
		},
		{Role: RoleTool, ToolCallID: "call_1", Content: "package main"},
	}

	if _, err := c.Complete(context.Background(), history); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(gotBody.Messages) != 3 {
		t.Fatalf("request has %d messages, want 3", len(gotBody.Messages))
	}

	assistantMsg := gotBody.Messages[1]
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("assistant message has %d tool calls, want 1", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].ID != "call_1" || assistantMsg.ToolCalls[0].Function.Name != "read" {
		t.Errorf("assistant tool call = %+v, want id call_1, function read", assistantMsg.ToolCalls[0])
	}

	toolMsg := gotBody.Messages[2]
	if toolMsg.Role != "tool" {
		t.Errorf("tool message role = %q, want %q", toolMsg.Role, "tool")
	}
	if toolMsg.ToolCallID != "call_1" {
		t.Errorf("tool message tool_call_id = %q, want %q", toolMsg.ToolCallID, "call_1")
	}
}
