// Package openrouter implements provider.Provider against OpenRouter's
// chat-completions API via the official github.com/OpenRouterTeam/go-sdk.
package openrouter

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"

	openroutersdk "github.com/OpenRouterTeam/go-sdk"

	"github.com/mgoodness/liam/internal/provider"
)

// Provider is a provider.Provider backed by OpenRouter. It defaults to the
// "openrouter/auto" model when a Request doesn't specify one.
type Provider struct {
	client *openroutersdk.OpenRouter

	contextMu    sync.Mutex
	contextCache map[string]int // model id -> max context length, memoized by MaxContextLength
}

var _ provider.Provider = (*Provider)(nil)

// New builds a Provider authenticated with apiKey. Extra SDK options (e.g.
// openroutersdk.WithServerURL, for tests) are applied after the security
// option, so they can't accidentally drop the API key.
func New(apiKey string, opts ...openroutersdk.SDKOption) *Provider {
	return &Provider{
		client:       openroutersdk.New(clientOptions(apiKey, opts...)...),
		contextCache: map[string]int{},
	}
}

func (p *Provider) Name() string { return "openrouter" }

// Stream sends req and yields events as they arrive over OpenRouter's SSE
// stream. Text deltas are yielded as they're received; tool calls (streamed
// as argument fragments keyed by index) are accumulated and yielded whole
// once the stream ends; a DoneEvent always yields last on success.
func (p *Provider) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		chatReq := buildChatRequest(req)

		resp, err := p.client.Chat.Send(ctx, chatReq, nil)
		if err != nil {
			yield(nil, classifyError(err))
			return
		}
		if resp.EventStream == nil {
			yield(nil, &provider.ProviderError{
				Kind:  provider.ErrorKindUnknown,
				Cause: errors.New("openrouter: expected a streaming response but got none"),
			})
			return
		}
		stream := resp.EventStream
		defer stream.Close()

		type toolCallAcc struct {
			id, name, args string
		}
		var toolCallOrder []int64
		toolCalls := map[int64]*toolCallAcc{}

		var finishReason, modelUsed string
		var usage provider.Usage

		for stream.Next() {
			chunk := stream.Value().Data

			if chunk.Error != nil {
				yield(nil, &provider.ProviderError{
					Kind:  provider.ErrorKindUnknown,
					Cause: fmt.Errorf("openrouter: %s", chunk.Error.Message),
				})
				return
			}
			if chunk.Model != "" {
				modelUsed = chunk.Model
			}
			if chunk.Usage != nil {
				usage = convertUsage(chunk.Usage)
			}

			for _, choice := range chunk.Choices {
				if content, ok := choice.Delta.GetContent().GetOrZero(); ok && content != "" {
					if !yield(provider.TextDeltaEvent{Text: content}, nil) {
						return
					}
				}

				for _, tc := range choice.Delta.GetToolCalls() {
					acc, seen := toolCalls[tc.Index]
					if !seen {
						acc = &toolCallAcc{}
						toolCalls[tc.Index] = acc
						toolCallOrder = append(toolCallOrder, tc.Index)
					}
					if tc.ID != nil {
						acc.id += *tc.ID
					}
					if fn := tc.Function; fn != nil {
						if fn.Name != nil {
							acc.name += *fn.Name
						}
						if fn.Arguments != nil {
							acc.args += *fn.Arguments
						}
					}
				}

				if choice.FinishReason != nil {
					finishReason = string(*choice.FinishReason)
				}
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, classifyError(err))
			return
		}

		for _, idx := range toolCallOrder {
			acc := toolCalls[idx]
			if !yield(provider.ToolCallEvent{ID: acc.id, Name: acc.name, ArgsJSON: acc.args}, nil) {
				return
			}
		}

		yield(provider.DoneEvent{
			FinishReason: finishReason,
			ModelUsed:    modelUsed,
			Usage:        usage,
		}, nil)
	}
}
