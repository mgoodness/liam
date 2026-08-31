package openrouter

import (
	"errors"
	"testing"

	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/models/sdkerrors"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"

	"github.com/mgoodness/liam/internal/provider"
)

func TestBuildChatRequestDefaultsModel(t *testing.T) {
	req := buildChatRequest(provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hi"}},
	})

	if req.Model == nil || *req.Model != defaultModel {
		t.Fatalf("Model = %v, want %q", req.Model, defaultModel)
	}
	if req.Stream == nil || !*req.Stream {
		t.Fatalf("Stream = %v, want true", req.Stream)
	}
}

func TestBuildChatRequestPassesThroughExplicitModel(t *testing.T) {
	req := buildChatRequest(provider.Request{Model: "openai/gpt-4o"})

	if req.Model == nil || *req.Model != "openai/gpt-4o" {
		t.Fatalf("Model = %v, want %q", req.Model, "openai/gpt-4o")
	}
}

func TestBuildChatRequestPrependsSystemPrompt(t *testing.T) {
	req := buildChatRequest(provider.Request{
		SystemPrompt: "be terse",
		Messages:     []provider.Message{{Role: "user", Content: "hi"}},
	})

	if len(req.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(req.Messages))
	}
	sys := req.Messages[0].ChatSystemMessage
	if sys == nil {
		t.Fatalf("Messages[0] is not a system message: %+v", req.Messages[0])
	}
	if sys.Content.Str == nil || *sys.Content.Str != "be terse" {
		t.Fatalf("system content = %v, want %q", sys.Content.Str, "be terse")
	}
}

func TestToChatMessageRoles(t *testing.T) {
	cases := []struct {
		name string
		msg  provider.Message
		want func(components.ChatMessages) bool
	}{
		{
			name: "user",
			msg:  provider.Message{Role: "user", Content: "hi"},
			want: func(m components.ChatMessages) bool {
				return m.ChatUserMessage != nil && *m.ChatUserMessage.Content.Str == "hi"
			},
		},
		{
			name: "assistant",
			msg:  provider.Message{Role: "assistant", Content: "hello"},
			want: func(m components.ChatMessages) bool {
				if m.ChatAssistantMessage == nil {
					return false
				}
				content, ok := m.ChatAssistantMessage.Content.GetOrZero()
				return ok && content.Str != nil && *content.Str == "hello"
			},
		},
		{
			name: "tool",
			msg:  provider.Message{Role: "tool", Content: "result", ToolCallID: "call_1"},
			want: func(m components.ChatMessages) bool {
				return m.ChatToolMessage != nil &&
					*m.ChatToolMessage.Content.Str == "result" &&
					m.ChatToolMessage.ToolCallID == "call_1"
			},
		},
		{
			name: "unrecognized role falls back to user",
			msg:  provider.Message{Role: "system", Content: "hi"},
			want: func(m components.ChatMessages) bool {
				return m.ChatUserMessage != nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toChatMessage(tc.msg)
			if !tc.want(got) {
				t.Fatalf("toChatMessage(%+v) = %+v, did not match expectation", tc.msg, got)
			}
		})
	}
}

func TestToChatToolsEmpty(t *testing.T) {
	if got := toChatTools(nil); got != nil {
		t.Fatalf("toChatTools(nil) = %v, want nil", got)
	}
}

func TestToChatToolsConvertsFields(t *testing.T) {
	tools := toChatTools([]provider.ToolDef{{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  map[string]any{"type": "object"},
	}})

	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	fn := tools[0].ChatFunctionToolFunction
	if fn == nil {
		t.Fatalf("tools[0].ChatFunctionToolFunction is nil")
	}
	if fn.Function.Name != "read_file" {
		t.Errorf("Name = %q, want %q", fn.Function.Name, "read_file")
	}
	if fn.Function.Description == nil || *fn.Function.Description != "Read a file" {
		t.Errorf("Description = %v, want %q", fn.Function.Description, "Read a file")
	}
	if fn.Function.Parameters["type"] != "object" {
		t.Errorf("Parameters[type] = %v, want %q", fn.Function.Parameters["type"], "object")
	}
}

func TestConvertUsage(t *testing.T) {
	cached := int64(3)
	cost := 0.0042
	usage := convertUsage(&components.ChatUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		Cost:             optionalnullable.From(&cost),
		PromptTokensDetails: optionalnullable.From(&components.ChatUsagePromptTokensDetails{
			CachedTokens: &cached,
		}),
	})

	want := provider.Usage{
		InputTokens:       10,
		OutputTokens:      5,
		CachedInputTokens: 3,
		CostUSD:           0.0042,
	}
	if usage != want {
		t.Fatalf("convertUsage() = %+v, want %+v", usage, want)
	}
}

func TestConvertUsageNil(t *testing.T) {
	if got := convertUsage(nil); got != (provider.Usage{}) {
		t.Fatalf("convertUsage(nil) = %+v, want zero value", got)
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want provider.ErrorKind
	}{
		{"rate limited", &sdkerrors.TooManyRequestsResponseError{}, provider.ErrorKindRateLimited},
		{"bad request", &sdkerrors.BadRequestResponseError{}, provider.ErrorKindInvalidRequest},
		{"unauthorized", &sdkerrors.UnauthorizedResponseError{}, provider.ErrorKindInvalidRequest},
		{"payment required", &sdkerrors.PaymentRequiredResponseError{}, provider.ErrorKindInvalidRequest},
		{"payload too large", &sdkerrors.PayloadTooLargeResponseError{}, provider.ErrorKindContextTooLong},
		{"service unavailable", &sdkerrors.ServiceUnavailableResponseError{}, provider.ErrorKindUnavailable},
		{"generic 500", sdkerrors.NewAPIError("boom", 500, "", nil), provider.ErrorKindUnavailable},
		{"generic 429", sdkerrors.NewAPIError("boom", 429, "", nil), provider.ErrorKindRateLimited},
		{"generic 400", sdkerrors.NewAPIError("boom", 400, "", nil), provider.ErrorKindInvalidRequest},
		{"unknown", errors.New("network blip"), provider.ErrorKindUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got.Kind != tc.want {
				t.Fatalf("classifyError(%v).Kind = %v, want %v", tc.err, got.Kind, tc.want)
			}
			if !errors.Is(got, tc.err) {
				t.Fatalf("classifyError(%v) does not unwrap to the original error", tc.err)
			}
		})
	}
}
