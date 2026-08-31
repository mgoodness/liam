package openrouter

import (
	openroutersdk "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/models/sdkerrors"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"

	"github.com/mgoodness/liam/internal/provider"
)

const defaultModel = "openrouter/auto"

func buildChatRequest(req provider.Request) components.ChatRequest {
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	stream := true

	var messages []components.ChatMessages
	if req.SystemPrompt != "" {
		messages = append(messages, components.CreateChatMessagesSystem(components.ChatSystemMessage{
			Content: components.CreateChatSystemMessageContentStr(req.SystemPrompt),
		}))
	}
	for _, m := range req.Messages {
		messages = append(messages, toChatMessage(m))
	}

	return components.ChatRequest{
		Model:    &model,
		Messages: messages,
		Stream:   &stream,
		Tools:    toChatTools(req.Tools),
	}
}

func toChatMessage(m provider.Message) components.ChatMessages {
	switch m.Role {
	case "assistant":
		content := components.CreateChatAssistantMessageContentStr(m.Content)
		return components.CreateChatMessagesAssistant(components.ChatAssistantMessage{
			Content:   optionalnullable.From(&content),
			ToolCalls: toChatToolCalls(m.ToolCalls),
		})
	case "tool":
		return components.CreateChatMessagesTool(components.ChatToolMessage{
			Content:    components.CreateChatToolMessageContentStr(m.Content),
			ToolCallID: m.ToolCallID,
		})
	default: // "user" and anything unrecognized falls back to user
		return components.CreateChatMessagesUser(components.ChatUserMessage{
			Content: components.CreateChatUserMessageContentStr(m.Content),
		})
	}
}

// toChatToolCalls reconstructs the tool_calls array OpenRouter's API
// requires on an assistant message that requested tool calls, from the
// provider.ToolCalls a prior turn accumulated onto that Message.
func toChatToolCalls(calls []provider.ToolCall) []components.ChatToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]components.ChatToolCall, 0, len(calls))
	for _, c := range calls {
		out = append(out, components.ChatToolCall{
			ID:   c.ID,
			Type: components.ChatToolCallTypeFunction,
			Function: components.ChatToolCallFunction{
				Name:      c.Name,
				Arguments: c.ArgsJSON,
			},
		})
	}
	return out
}

func toChatTools(tools []provider.ToolDef) []components.ChatFunctionTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]components.ChatFunctionTool, 0, len(tools))
	for _, t := range tools {
		description := t.Description
		out = append(out, components.CreateChatFunctionToolChatFunctionToolFunction(components.ChatFunctionToolFunction{
			Type: components.ChatFunctionToolTypeFunction,
			Function: components.ChatFunctionToolFunctionFunction{
				Name:        t.Name,
				Description: &description,
				Parameters:  t.Parameters,
			},
		}))
	}
	return out
}

func convertUsage(u *components.ChatUsage) provider.Usage {
	if u == nil {
		return provider.Usage{}
	}
	var cached int
	if details, ok := u.GetPromptTokensDetails().GetOrZero(); ok && details.CachedTokens != nil {
		cached = int(*details.CachedTokens)
	}
	var cost float64
	if c, ok := u.GetCost().GetOrZero(); ok {
		cost = c
	}
	return provider.Usage{
		InputTokens:       int(u.PromptTokens),
		OutputTokens:      int(u.CompletionTokens),
		CachedInputTokens: cached,
		CostUSD:           cost,
	}
}

// classifyError maps an error returned by the OpenRouter SDK to a
// provider.ProviderError, so callers never need to know the SDK's own
// exported error types.
func classifyError(err error) *provider.ProviderError {
	switch e := err.(type) {
	case *sdkerrors.TooManyRequestsResponseError:
		return &provider.ProviderError{Kind: provider.ErrorKindRateLimited, Cause: err}
	case *sdkerrors.BadRequestResponseError,
		*sdkerrors.UnauthorizedResponseError,
		*sdkerrors.PaymentRequiredResponseError,
		*sdkerrors.ForbiddenResponseError,
		*sdkerrors.NotFoundResponseError,
		*sdkerrors.UnprocessableEntityResponseError:
		return &provider.ProviderError{Kind: provider.ErrorKindInvalidRequest, Cause: err}
	case *sdkerrors.PayloadTooLargeResponseError:
		return &provider.ProviderError{Kind: provider.ErrorKindContextTooLong, Cause: err}
	case *sdkerrors.RequestTimeoutResponseError,
		*sdkerrors.InternalServerResponseError,
		*sdkerrors.BadGatewayResponseError,
		*sdkerrors.ServiceUnavailableResponseError,
		*sdkerrors.EdgeNetworkTimeoutResponseError,
		*sdkerrors.ProviderOverloadedResponseError:
		return &provider.ProviderError{Kind: provider.ErrorKindUnavailable, Cause: err}
	case *sdkerrors.APIError:
		switch {
		case e.StatusCode == 429:
			return &provider.ProviderError{Kind: provider.ErrorKindRateLimited, Cause: err}
		case e.StatusCode >= 500:
			return &provider.ProviderError{Kind: provider.ErrorKindUnavailable, Cause: err}
		case e.StatusCode >= 400:
			return &provider.ProviderError{Kind: provider.ErrorKindInvalidRequest, Cause: err}
		}
	}
	return &provider.ProviderError{Kind: provider.ErrorKindUnknown, Cause: err}
}

// clientOptions is split out purely so tests can override the SDK's server
// URL (via openroutersdk.WithServerURL) while still going through New.
func clientOptions(apiKey string, extra ...openroutersdk.SDKOption) []openroutersdk.SDKOption {
	return append([]openroutersdk.SDKOption{openroutersdk.WithSecurity(apiKey)}, extra...)
}
