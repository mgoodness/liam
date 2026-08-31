// Package tool defines the Tool interface the agent loop dispatches
// model-requested tool calls to, plus the harness's built-in tools.
package tool

import "context"

// Tool is a single function the model can invoke mid-conversation.
type Tool interface {
	Name() string
	Description() string
	Parameters() Schema
	Safety() Safety
	Run(ctx context.Context, args map[string]any) Result
}

// Schema is a tool's parameters, expressed as JSON Schema.
type Schema map[string]any

// SideEffect classifies the kind of effect a Tool's Run can have.
type SideEffect string

const (
	SideEffectRead    SideEffect = "read"
	SideEffectWrite   SideEffect = "write"
	SideEffectShell   SideEffect = "shell"
	SideEffectNetwork SideEffect = "network"
)

// Safety classifies a Tool's effect. liam ships no built-in permission
// system (see ADR-0004) — built-in tools run with the harness process's own
// permissions; SideEffect exists for Trace's audit categorization, not for
// gating.
type Safety struct {
	SideEffect SideEffect
}

// Result is what a Tool's Run call reports back to the agent loop.
type Result struct {
	// Content is threaded back into the conversation as a "tool"-role
	// Message's Content.
	Content string
	// IsError marks Content as a failure description rather than normal
	// output. The harness never retries a failed tool call itself — this
	// only lets the model see the call failed so it can decide what to do.
	IsError bool
}

// errorResult builds a failed Result from a formatted message.
func errorResult(msg string) Result {
	return Result{Content: msg, IsError: true}
}
