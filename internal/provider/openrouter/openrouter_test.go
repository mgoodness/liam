package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openroutersdk "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"

	"github.com/mgoodness/liam/internal/provider"
)

func TestName(t *testing.T) {
	p := New("test-key")
	if got, want := p.Name(), "openrouter"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func sseServer(t *testing.T, checkRequest func(*testing.T, *http.Request, []byte), chunks []components.ChatStreamChunk) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if checkRequest != nil {
			checkRequest(t, r, body)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			data, err := json.Marshal(c)
			if err != nil {
				t.Fatalf("marshal chunk: %v", err)
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

func newTestProvider(serverURL string) *Provider {
	return New("test-key", openroutersdk.WithServerURL(serverURL))
}

func strPtr(s string) *string { return &s }

func finishReason(r components.ChatFinishReasonEnum) *components.ChatFinishReasonEnum { return &r }

func TestStreamYieldsTextDeltasThenDone(t *testing.T) {
	chunks := []components.ChatStreamChunk{
		{
			ID: "chunk-1", Object: components.ChatStreamChunkObjectChatCompletionChunk, Model: "openrouter/auto",
			Choices: []components.ChatStreamChoice{{
				Index: 0,
				Delta: components.ChatStreamDelta{Content: optionalFrom("Hello, ")},
			}},
		},
		{
			ID: "chunk-2", Object: components.ChatStreamChunkObjectChatCompletionChunk, Model: "openrouter/auto",
			Choices: []components.ChatStreamChoice{{
				Index:        0,
				Delta:        components.ChatStreamDelta{Content: optionalFrom("world!")},
				FinishReason: finishReason(components.ChatFinishReasonEnumStop),
			}},
			Usage: &components.ChatUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
		},
	}

	var sawAuth, sawStream bool
	srv := sseServer(t, func(t *testing.T, r *http.Request, body []byte) {
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body: %v (body=%s)", err, body)
		}
		sawStream, _ = payload["stream"].(bool)
		if payload["model"] != defaultModel {
			t.Errorf("request model = %v, want %q", payload["model"], defaultModel)
		}
	}, chunks)
	defer srv.Close()

	p := newTestProvider(srv.URL)
	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}

	var texts []string
	var done *provider.DoneEvent
	for ev, err := range p.Stream(context.Background(), req) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		switch e := ev.(type) {
		case provider.TextDeltaEvent:
			texts = append(texts, e.Text)
		case provider.DoneEvent:
			done = &e
		default:
			t.Fatalf("unexpected event %T", ev)
		}
	}

	if !sawAuth {
		t.Error("request did not carry the expected Authorization header")
	}
	if !sawStream {
		t.Error("request did not set stream=true")
	}
	if got, want := strings.Join(texts, ""), "Hello, world!"; got != want {
		t.Errorf("streamed text = %q, want %q", got, want)
	}
	if done == nil {
		t.Fatal("did not receive a DoneEvent")
	}
	if done.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", done.FinishReason, "stop")
	}
	if done.ModelUsed != "openrouter/auto" {
		t.Errorf("ModelUsed = %q, want %q", done.ModelUsed, "openrouter/auto")
	}
	if done.Usage.InputTokens != 10 || done.Usage.OutputTokens != 4 {
		t.Errorf("Usage = %+v, want InputTokens=10 OutputTokens=4", done.Usage)
	}
}

func TestStreamAccumulatesToolCallsAcrossChunks(t *testing.T) {
	chunks := []components.ChatStreamChunk{
		{
			Object: components.ChatStreamChunkObjectChatCompletionChunk, Model: "openrouter/auto",
			Choices: []components.ChatStreamChoice{{
				Index: 0,
				Delta: components.ChatStreamDelta{ToolCalls: []components.ChatStreamToolCall{{
					Index: 0,
					ID:    strPtr("call_1"),
					Function: &components.ChatStreamToolCallFunction{
						Name:      strPtr("read_file"),
						Arguments: strPtr(`{"path":`),
					},
				}}},
			}},
		},
		{
			Object: components.ChatStreamChunkObjectChatCompletionChunk, Model: "openrouter/auto",
			Choices: []components.ChatStreamChoice{{
				Index: 0,
				Delta: components.ChatStreamDelta{ToolCalls: []components.ChatStreamToolCall{{
					Index: 0,
					Function: &components.ChatStreamToolCallFunction{
						Arguments: strPtr(`"a.go"}`),
					},
				}}},
				FinishReason: finishReason(components.ChatFinishReasonEnumToolCalls),
			}},
		},
	}

	srv := sseServer(t, nil, chunks)
	defer srv.Close()

	p := newTestProvider(srv.URL)
	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "read a.go"}}}

	var toolCall *provider.ToolCallEvent
	for ev, err := range p.Stream(context.Background(), req) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tc, ok := ev.(provider.ToolCallEvent); ok {
			toolCall = &tc
		}
	}

	if toolCall == nil {
		t.Fatal("did not receive a ToolCallEvent")
	}
	if toolCall.ID != "call_1" {
		t.Errorf("ID = %q, want %q", toolCall.ID, "call_1")
	}
	if toolCall.Name != "read_file" {
		t.Errorf("Name = %q, want %q", toolCall.Name, "read_file")
	}
	if toolCall.ArgsJSON != `{"path":"a.go"}` {
		t.Errorf("ArgsJSON = %q, want %q", toolCall.ArgsJSON, `{"path":"a.go"}`)
	}
}

func TestStreamMapsHTTPErrorToProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":429,"message":"rate limited"}}`)
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hi"}}}

	var gotErr error
	for _, err := range p.Stream(context.Background(), req) {
		if err != nil {
			gotErr = err
			break
		}
	}

	if gotErr == nil {
		t.Fatal("expected an error, got none")
	}
	perr, ok := gotErr.(*provider.ProviderError)
	if !ok {
		t.Fatalf("error %v (%T) is not a *provider.ProviderError", gotErr, gotErr)
	}
	if perr.Kind != provider.ErrorKindRateLimited {
		t.Errorf("Kind = %v, want %v", perr.Kind, provider.ErrorKindRateLimited)
	}
}

func optionalFrom(s string) optionalnullable.OptionalNullable[string] {
	return optionalnullable.From(&s)
}
