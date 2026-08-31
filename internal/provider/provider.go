// Package provider defines the harness's abstraction over model backends.
package provider

import (
	"context"
	"fmt"
	"iter"
)

// Provider is a backend that serves model completions.
type Provider interface {
	Name() string
	Stream(ctx context.Context, req Request) iter.Seq2[Event, error]
}

// Request is one turn's worth of input to a Provider.
type Request struct {
	// Model is an opaque passthrough, e.g. "openrouter/auto". A Provider may
	// apply its own default when empty.
	Model        string
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDef
}

// Message is one entry in a Request's conversation history.
type Message struct {
	Role    string // "user" | "assistant" | "tool"
	Content string
	// ToolCallID is set on "tool"-role messages carrying a result back in.
	ToolCallID string
}

// ToolDef describes a tool the model may call, in provider-agnostic form.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
}

// Event is one item streamed back from a Provider during a turn.
type Event interface{ isEvent() }

// TextDeltaEvent carries one fragment of streamed assistant text.
type TextDeltaEvent struct{ Text string }

func (TextDeltaEvent) isEvent() {}

// ToolCallEvent signals the model has requested a tool call.
type ToolCallEvent struct{ ID, Name, ArgsJSON string }

func (ToolCallEvent) isEvent() {}

// DoneEvent marks the end of a turn.
type DoneEvent struct {
	FinishReason string
	ModelUsed    string
	Usage        Usage
}

func (DoneEvent) isEvent() {}

// Usage reports token and cost accounting for a turn.
type Usage struct {
	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
	CostUSD           float64
}

// ErrorKind classifies a ProviderError so callers can decide retry policy.
type ErrorKind string

const (
	ErrorKindRateLimited    ErrorKind = "rate_limited"
	ErrorKindInvalidRequest ErrorKind = "invalid_request"
	ErrorKindContextTooLong ErrorKind = "context_too_long"
	ErrorKindUnavailable    ErrorKind = "unavailable"
	ErrorKindUnknown        ErrorKind = "unknown"
)

// ProviderError wraps a Provider failure with a classification.
type ProviderError struct {
	Kind  ErrorKind
	Cause error
}

func (e *ProviderError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("provider error (%s)", e.Kind)
	}
	return fmt.Sprintf("provider error (%s): %v", e.Kind, e.Cause)
}

func (e *ProviderError) Unwrap() error { return e.Cause }
