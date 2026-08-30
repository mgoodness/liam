# MCP client requirements

Research for [Research: MCP client requirements](https://github.com/mgoodness/liam/issues/5), a ticket on the [liam: Go coding-agent harness — spec](https://github.com/mgoodness/liam/issues/1) map.

## Transports

The spec (2025-06-18) defines two standard transports ([source](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)):

- **stdio** — client launches the server as a subprocess; JSON-RPC messages flow over the child's stdin/stdout, newline-delimited; server may log to stderr. Clients **SHOULD** support this whenever possible.
- **Streamable HTTP** — server is a standalone HTTP process exposing one endpoint (e.g. `/mcp`) accepting both POST (client→server messages) and GET (opens an optional SSE stream for server→client push). A response to a POST may come back as a single JSON object or as an SSE stream. Supports resumable sessions via an `Mcp-Session-Id` header and per-stream event IDs for redelivery after disconnect.
- The older **HTTP+SSE** transport (protocol version 2024-11-05, separate SSE-stream + POST-endpoint pair) is deprecated but documented for backwards compat: a client can probe by POSTing `initialize` to the given URL — a 4xx means "fall back to legacy SSE".
- Custom transports are explicitly allowed as long as they preserve JSON-RPC framing and the lifecycle below.

Clients **MUST** send `MCP-Protocol-Version: <version>` on every HTTP request after initialization.

## Lifecycle & capability negotiation

Three phases: **Initialization → Operation → Shutdown** ([source](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle)).

1. Client sends `initialize` (protocol version, its capabilities, client info).
2. Server responds with its own negotiated protocol version, capabilities, server info, and optional `instructions` text.
3. Client sends `notifications/initialized` — only then may normal operation messages flow (pings excepted).

Capabilities declared at this handshake gate what's usable for the rest of the session:

| Side | Capability | Meaning |
|---|---|---|
| Client | `roots` | can expose filesystem roots to the server |
| Client | `sampling` | server may ask the client to run an LLM completion on its behalf |
| Client | `elicitation` | server may ask the client to prompt the user mid-tool-call for more input |
| Server | `tools` | exposes callable tools |
| Server | `resources` | exposes readable resources (+ optional `subscribe`/`listChanged`) |
| Server | `prompts` | exposes prompt templates |
| Server | `logging`, `completions` | structured logs / argument autocomplete |

A minimal client aimed at "call tools from configured servers" only needs to declare no client capabilities (or `roots` if it wants to expose the workspace) and only needs to use the server's `tools` capability — `resources`/`prompts`/`sampling`/`elicitation` are all optional extensions, not required for a basic integration.

Shutdown: for stdio, client closes stdin, waits, escalates to SIGTERM then SIGKILL; for HTTP, close the connection(s).

## Official Go SDK

`github.com/modelcontextprotocol/go-sdk` — the official SDK, maintained in collaboration with Google, reached a stable **v1.0.0** with an API compatibility guarantee ([repo](https://github.com/modelcontextprotocol/go-sdk), [pkg.go.dev](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk)).

Client-side shape:

- `mcp.NewClient(impl, options)` → `*Client` — the entry point; can be configured with handlers for elicitation, progress notifications, etc.
- `client.Connect(ctx, transport, opts)` → `*ClientSession` — one active connection to one server.
- Transports available: `StdioTransport` / `CommandTransport` (subprocess), `SSEClientTransport`, `StreamableClientTransport` (HTTP), `InMemoryTransport` (testing/`NewInMemoryTransports()`).
- `ClientSession` methods: `CallTool`, `ListTools` (+ iterator-based `Tools`), `ReadResource`, `Resources`/`ResourceTemplates`, `GetPrompt`/`ListPrompts`, `Complete`.
- Package layout: `mcp` (main API), `jsonrpc` (for custom transports), `auth` (OAuth primitives), `oauthex` (OAuth extensions e.g. `ProtectedResourceMetadata`).

**Conclusion for liam**: a v1 MCP client only needs stdio transport + the `tools` capability (list + call) to satisfy "MCP server exposes tools to the model" — that's a thin layer over `go-sdk`'s `Client`/`ClientSession`/`StdioTransport`/`CallTool`/`ListTools`. HTTP transport, resources/prompts/sampling/elicitation, and OAuth are all real but separable v2+ extensions.

## Sources

- [MCP Transports spec (2025-06-18)](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)
- [MCP Lifecycle spec (2025-06-18)](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle)
- [MCP introduction](https://modelcontextprotocol.io/introduction)
- [modelcontextprotocol/go-sdk repo](https://github.com/modelcontextprotocol/go-sdk)
- [go-sdk mcp package docs](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)
- [go-sdk v1.0.0 release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.0.0)
