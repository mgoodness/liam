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

// Permission is the gating decision for a Tool call. The permission system
// itself — config-driven policy per SideEffect/tool, interactive prompting —
// is a later ticket; every built-in tool reports PermissionAllow as a no-op
// default until that system exists.
type Permission string

const (
	PermissionAllow  Permission = "allow"
	PermissionPrompt Permission = "prompt"
	PermissionDeny   Permission = "deny"
)

// Safety classifies a Tool's effect and the permission required to run it.
type Safety struct {
	SideEffect SideEffect
	Permission Permission
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
