package provider

import (
	"context"
	"encoding/json"
)

// Role identifies who authored a Message in the history passed to a
// Provider.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in the conversation history passed to a Provider:
// the system prompt, a user turn, a prior assistant turn (optionally
// carrying ToolCalls it requested), or a tool-result turn (ToolCallID
// identifying which call it answers).
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolCall is a single tool invocation requested by the model, identified
// by ID so its eventual result can be matched back to it via a Message's
// ToolCallID.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Response is the model's next turn: final assistant text, one or more
// tool calls to execute, or both together (per the agent loop's
// message-integrity rule, a response is never split across two history
// entries).
type Response struct {
	Text      string
	ToolCalls []ToolCall
}

// Provider abstracts a single LLM backend: given the running message
// history, return the model's next Response. Everything backend-specific
// — authentication, request/response shape, retries — is internal to the
// implementation and invisible here.
type Provider interface {
	Complete(ctx context.Context, messages []Message) (Response, error)
}
