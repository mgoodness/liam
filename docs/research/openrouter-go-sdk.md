# OpenRouter Go SDK & auto-routing

Findings for [liam#2](https://github.com/mgoodness/liam/issues/2).

## Official Go SDK exists

OpenRouter ships an **official** Go SDK: [github.com/OpenRouterTeam/go-sdk](https://github.com/OpenRouterTeam/go-sdk) (`go get github.com/OpenRouterTeam/go-sdk`).

- Requires **Go 1.25+**.
- Still in **beta**: the README warns of breaking changes in minor `0.x` releases and recommends pinning to a specific version (v0.7.97+ per the README at fetch time).
- Community alternatives also exist (`revrost/go-openrouter`, `mshafiee/openrouter-go`, `eduardolat/openroutergo`, `wojtess/openrouter-api-go`) but the official SDK is the natural default given the user's stated preference for the client SDK.

Source: [github.com/OpenRouterTeam/go-sdk](https://github.com/OpenRouterTeam/go-sdk), [pkg.go.dev/github.com/OpenRouterTeam/go-sdk](https://pkg.go.dev/github.com/OpenRouterTeam/go-sdk)

## Client construction & chat completions

```go
s := openrouter.New(
    openrouter.WithSecurity(os.Getenv("OPENROUTER_API_KEY")),
)

res, err := s.Chat.Send(ctx, components.ChatCompletionRequest{
    Model: "openai/gpt-4o",
    Messages: []components.Message{{Role: "user", Content: "prompt"}},
})
```

Source: [openrouter.ai/docs/client-sdks/go/overview](https://openrouter.ai/docs/client-sdks/go/overview), [github.com/OpenRouterTeam/go-sdk README](https://github.com/OpenRouterTeam/go-sdk)

## Streaming

The REST API supports SSE streaming for all models via `stream: true`. The Go SDK exposes a streaming pattern at least for the `Responses` endpoint (`s.Responses.Send` returns a value with `.Next()` / `.Value()` / `.Close()`); the docs fetched didn't show a `Chat.Send` streaming example specifically, so **implementers should verify the exact streaming call shape against the SDK source when writing the Provider interface (ticket #13)**.

Source: [openrouter.ai/docs/api-reference/overview](https://openrouter.ai/docs/api-reference/overview), [github.com/OpenRouterTeam/go-sdk README](https://github.com/OpenRouterTeam/go-sdk)

## Tool / function calling

The REST API's `chat/completions` request takes `tools` and `tool_choice`; OpenRouter normalizes/transforms tool-call requests across providers (including converting to YAML templates for providers without native tool support). Responses include `tool_calls` in `choices[].message` when the model requests a function call, with normalized `finish_reason: "tool_calls"`.

The fetched Go SDK docs/README didn't surface an explicit tool-calling code example — **verify the exact Go types (`components.ChatRequest.Tools`, expected schema shape) against SDK source when designing the Provider interface (ticket #13)**.

Source: [openrouter.ai/docs/api-reference/overview](https://openrouter.ai/docs/api-reference/overview)

## Auto-routing mode

Set `model: "openrouter/auto"` (beta variant: `"openrouter/auto-beta"`, which requires plugin id `"auto-beta-router"` instead of `"auto-router"`).

**Selection mechanism** (4 steps):
1. **Task classification** — a lightweight classifier assigns the prompt to one of ~30 task types (e.g. `code:debugging`, `math`, `customer_support`).
2. **Market-based ranking** — ranks candidate models by real-world spend from OpenRouter's community over the trailing 7 days for that task type.
3. **Cost-tier filtering** — a chosen cost band (`low`/`medium`/`high`/`xhigh`/`max`) filters candidates; "a tier is a band, not a ceiling."
4. **Fallback selection** — top candidate plus fallbacks, respecting account restrictions and output requirements.

**Behavioral notes:**
- **Session stickiness**: the router remembers which model a conversation landed on and prefers it on later turns in the same session.
- **Transparency**: every response's `model` field reports which model actually answered — important for liam's UX (ticket #20, "OpenRouter auto-routing UX in liam").
- **Cost**: no Auto Router premium — you pay standard per-model rates for whichever model gets picked. `provider.max_price` can cap request cost.
- **Latency**: no added latency penalty beyond normal model selection; streaming and all standard features work with auto-routed models.

Source: [openrouter.ai/docs/features/model-routing](https://openrouter.ai/docs/features/model-routing)
