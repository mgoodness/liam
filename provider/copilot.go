package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/mgoodness/liam/tool"
)

// defaultModel is used when Copilot is constructed with no model override.
const defaultModel = "gpt-4o"

const defaultEndpoint = "https://api.githubcopilot.com/chat/completions"

// api.githubcopilot.com is Copilot Chat's internal API (see ADR 0001): it
// rejects requests that don't identify themselves as a known IDE client,
// so every request spoofs the same values VS Code's Copilot Chat
// extension sends. Fixed across every known third-party client, same as
// defaultClientID in auth.go — not a per-deployment setting.
const (
	copilotIntegrationID = "vscode-chat"
	editorVersion        = "vscode/1.85.1"
	editorPluginVersion  = "copilot-chat/0.12.0"
)

// Copilot implements Provider against Copilot Chat's internal
// chat-completions API (see ADR 0001). Authentication — the GitHub OAuth
// token vs. Copilot token distinction, device-flow login, refresh — is
// entirely internal, via Authenticator.
type Copilot struct {
	Authenticator *Authenticator
	HTTPClient    *http.Client
	Endpoint      string // overrides defaultEndpoint; mainly for tests
	Model         string
	Tools         []tool.Definition
}

// NewCopilot returns a Copilot provider authenticating via auth, offering
// tools to the model on every request. model overrides defaultModel when
// non-empty.
func NewCopilot(auth *Authenticator, model string, tools []tool.Definition) *Copilot {
	if model == "" {
		model = defaultModel
	}
	return &Copilot{
		Authenticator: auth,
		HTTPClient:    http.DefaultClient,
		Endpoint:      defaultEndpoint,
		Model:         model,
		Tools:         tools,
	}
}

// chatRequest is the OpenAI-chat-completions-shaped request body Copilot
// Chat's internal API expects.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
}

// chatTool wraps a tool.Definition in the "type": "function" envelope the
// chat-completions tools array expects.
type chatTool struct {
	Type     string          `json:"type"`
	Function tool.Definition `json:"function"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// chatToolCall mirrors the OpenAI-chat-completions tool-call shape: a
// call ID plus a "function" invocation whose arguments arrive as a
// JSON-encoded string (not a nested object), which Response.ToolCalls
// re-parses into json.RawMessage.
type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

// errAuthRejected marks a chat-completion request that api.githubcopilot.com
// rejected with an auth error (401) — distinct from a non-auth API error,
// which Complete returns to the caller as-is rather than retrying.
var errAuthRejected = errors.New("copilot api rejected the request as unauthorized")

// Complete implements Provider. On an auth error from the chat-completion
// call, it forces a full device-flow re-login (via Authenticator.Login)
// and retries the request exactly once; any other error — network
// failure, rate limit, or a second auth rejection after re-login — is
// returned to the caller without further retry.
func (c *Copilot) Complete(ctx context.Context, messages []Message) (Response, error) {
	creds, err := c.Authenticator.Authenticate(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("authenticating: %w", err)
	}

	resp, err := c.complete(ctx, creds.CopilotToken, messages)
	if errors.Is(err, errAuthRejected) {
		creds, err = c.Authenticator.Login(ctx)
		if err != nil {
			return Response{}, fmt.Errorf("re-authenticating: %w", err)
		}
		resp, err = c.complete(ctx, creds.CopilotToken, messages)
	}
	if err != nil {
		return Response{}, err
	}
	return resp, nil
}

func (c *Copilot) complete(ctx context.Context, copilotToken string, messages []Message) (Response, error) {
	reqBody := chatRequest{
		Model:    c.Model,
		Messages: toChatMessages(messages),
		Tools:    toChatTools(c.Tools),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return Response{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+copilotToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Copilot-Integration-Id", copilotIntegrationID)
	req.Header.Set("Editor-Version", editorVersion)
	req.Header.Set("Editor-Plugin-Version", editorPluginVersion)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return Response{}, errAuthRejected
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("unexpected status %s: %s", resp.Status, body)
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, err
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("chat completion response has no choices")
	}

	msg := out.Choices[0].Message
	return Response{Text: msg.Content, ToolCalls: toToolCalls(msg.ToolCalls)}, nil
}

func toToolCalls(calls []chatToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, len(calls))
	for i, c := range calls {
		out[i] = ToolCall{
			ID:        c.ID,
			Name:      c.Function.Name,
			Arguments: json.RawMessage(c.Function.Arguments),
		}
	}
	return out
}

func toChatTools(tools []tool.Definition) []chatTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]chatTool, len(tools))
	for i, t := range tools {
		out[i] = chatTool{Type: "function", Function: t}
	}
	return out
}

func toChatMessages(messages []Message) []chatMessage {
	out := make([]chatMessage, len(messages))
	for i, m := range messages {
		out[i] = chatMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCalls:  fromToolCalls(m.ToolCalls),
			ToolCallID: m.ToolCallID,
		}
	}
	return out
}

func fromToolCalls(calls []ToolCall) []chatToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]chatToolCall, len(calls))
	for i, c := range calls {
		out[i] = chatToolCall{
			ID:   c.ID,
			Type: "function",
			Function: chatFunctionCall{
				Name:      c.Name,
				Arguments: string(c.Arguments),
			},
		}
	}
	return out
}
