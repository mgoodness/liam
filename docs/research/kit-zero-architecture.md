# Research: kit's and zero's Go tool interface & agent-loop architecture

Ticket: [Research: kit's and zero's Go-level tool interface & agent-loop architecture](https://github.com/mgoodness/liam/issues/10)

Both repos verified as real, active Go coding-agent CLIs matching what was named:

- `github.com/mark3labs/kit` — cloned at commit visible via `git log -1` in a shallow clone taken 2026-08-30. Go module `github.com/mark3labs/kit`.
- `github.com/Gitlawb/zero` — cloned the same day. Go module `github.com/Gitlawb/zero`, requires Go 1.26.6+ per its README.

## kit: tool interface

Source: `internal/core/tools.go`, `internal/core/grep.go` (and siblings `bash.go`, `read.go`, `write.go`, `edit.go`, `find.go`, `ls.go`, `subagent.go` — all in `internal/core/`).

- kit does **not** define its own Tool interface. It builds on **`charm.land/fantasy`** (`go.mod` line 8, pinned `v0.41.3`), a third-party Go library (from Charm, the Bubbletea authors) that defines `fantasy.AgentTool`, `fantasy.ToolInfo`, `fantasy.ToolCall`, `fantasy.ToolResponse`, `fantasy.Agent`, `fantasy.LanguageModel`, `fantasy.AgentOption`, etc. — i.e. fantasy owns both the tool abstraction AND the provider/agent-loop primitives.
- A tool is a struct (`coreTool` in `internal/core/tools.go:79-93`) satisfying `fantasy.AgentTool` via four methods: `Info() fantasy.ToolInfo`, `ProviderOptions()`/`SetProviderOptions(...)`, and `Run(ctx, fantasy.ToolCall) (fantasy.ToolResponse, error)`.
- JSON schema is declared **by hand** as `map[string]any` literals inside `fantasy.ToolInfo{Parameters: ..., Required: ...}` (see `internal/core/grep.go:27-62`) — not generated from Go structs via reflection/tags. Each tool also defines a private `xArgs` struct (e.g. `grepArgs`) purely for `json.Unmarshal`-ing `call.Input` inside the handler (`parseArgs` helper, `tools.go:97-104`).
- Registration is a **package-level map + constructor functions**: `coreTools = map[string]initTool{"bash": NewBashTool, "read": NewReadTool, ...}` (`tools.go:113-122`). Convenience slices (`CodingTools()`, `ReadOnlyTools()`, `SubagentTools()`, `AllTools()`) return curated `[]fantasy.AgentTool` subsets built from that map; `ListedTools(names []string)` builds an arbitrary subset from user config.
- `ToolOption` (functional-options pattern: `WithWorkDir`, `WithBashTimeout`, etc.) configures shared behavior across constructors without changing each `NewXTool` signature.
- Tools run entirely **in-process**, no MCP/JSON-RPC involved — comment at top of `tools.go`: "direct fantasy.AgentTool implementations — no MCP layer, no JSON-RPC, no serialization overhead."

## kit: agent-loop architecture

Source: `internal/agent/agent.go` (1493 lines).

- kit's `Agent` struct (`agent.go:243`) wraps a `fantasy.Agent` (field `fantasyAgent`) — **the request/tool-call/feedback loop itself lives inside the `fantasy` library**, not in kit. kit's job is composing tools/options and exposing a wide callback surface.
- `NewAgent()` builds the LLM provider, composes tools (`composeAllTools()`, `agent.go:501`: core tools + MCP tools + "extra" tools, deduped by name via `dedupeToolsByName`, `agent.go:528`), and constructs a `fantasy.Agent` through `buildAgentOptions()` (`agent.go:545`) + `fantasy.NewAgent`-equivalent (referenced, not shown in this pass).
- Two entry points wrap `fantasyAgent`'s run: `GenerateWithLoop` (`agent.go:595`, simple) and `GenerateWithCallbacks` (`agent.go:611`, the real workhorse) — the latter accepts a `GenerateCallbacks` struct (`agent.go:204`) with 20+ handler types (`ToolCallHandler`, `StepStartHandler`, `StepFinishHandler`, `ReasoningDeltaHandler`, `RetryHandler`, `PrepareStepHandler`, etc.) so the TUI/CLI can render streaming/step/tool events without kit's agent package knowing about the UI.
- MCP tools load **asynchronously in the background**; the agent is usable immediately with core tools only, and the first `GenerateWithLoop`/`GenerateWithCallbacks` call blocks on `WaitForMCPTools()` (`agent.go:435`) before proceeding — avoids blocking startup on slow MCP servers.
- Tool set can change **mid-session**: `SetExtraTools` (`agent.go:1253`), `AddMCPServer`/`RemoveMCPServer` (`agent.go:1267`, `1297`) trigger `rebuildFantasyAgent()` (`agent.go:479`) to rebuild the underlying `fantasy.Agent` with the new tool list; a `toolsMu sync.RWMutex` guards `extraTools` so an in-flight `Stream` can re-read the live set per step.

## zero: tool interface

Source: `internal/tools/types.go`.

- zero defines its **own** `Tool` interface (no external agent framework):
  ```go
  type Tool interface {
      Name() string
      Description() string
      Parameters() Schema
      Safety() Safety
      Run(ctx context.Context, args map[string]any) Result
  }
  ```
- `Schema`/`PropertySchema` (`types.go:74-91`) are zero's own hand-rolled JSON-Schema-shaped structs (`type`, `properties`, `required`, `enum`, `minimum`/`maximum`, nested `Properties` for objects) — not generated from tags, built by hand per tool (same spirit as kit's `map[string]any`, but statically typed).
- The standout difference from kit: **permission/safety is part of the Tool interface itself**, not bolted on externally. `Safety` (`types.go:66-72`) carries `SideEffect` (`read`/`write`/`shell`/`network`/`local_browser`/`out_of_workspace`/...) and `Permission` (`allow`/`prompt`/`deny`), plus an `AdvertiseInAuto` flag for tools visible in auto-approve mode despite needing a prompt. Two optional interfaces let a tool refine this dynamically: `ArgsPermissioner.PermissionForArgs(args)` (relax permission for provably-safe arguments) and `PrePermissionRejecter.RejectBeforePermission(args)` (reject before any prompt, must be pure/local/deterministic).
- `baseTool` is an embeddable struct implementing the four getter methods (`Name`/`Description`/`Parameters`/`Safety`) so concrete tools only need to implement `Run`.
- `Result` (`types.go:96-140`) is richer than a plain string: `Status` (ok/error), `Output` (model-facing text), `Images` (delivered as a follow-up user message, not the tool-result block, because "every provider drops images there"), `ChangedFiles`, `Display` (a separate `Summary`/`Kind`/`Preview` for the TUI that costs **zero model tokens** since `Preview` is never sent to the model), and a `ToolOutcome` layer (`ModelView` vs `HumanView` vs `Artifact`) that's the canonical post-execution representation once a tool crosses "the registry boundary."
- Built-in tools live in `internal/tools/*.go`: `bash.go`, `edit_file.go`, `apply_patch.go`, `grep.go`, `exec_command.go`, `web_fetch.go`, `view_image.go`, `ask_user.go`, `escalate_model.go`, `request_permissions.go`, `lsp_navigate.go`, `local_terminal.go`, `local_browser.go`, `local_capture.go`, `tool_search.go`, plus a `registry.go`.

## zero: agent-loop architecture

Source: `internal/agent/loop.go` (3487 lines — large, hand-rolled, monolithic).

- Single entry point: `func Run(ctx context.Context, prompt string, provider Provider, options Options) (result Result, err error)` (`loop.go:123`). No external agent-framework dependency (unlike kit/fantasy) — zero owns the whole loop.
- `maxTurns` defaults to 12 if unset; a `tools.Registry` (default `tools.NewRegistry()`) supplies the active tool set; `PermissionMode` defaults to `PermissionModeAuto`.
- A `TurnSessionProvider` seam wraps the raw `Provider` so an optimized provider session (connection reuse, prewarm, native compaction) can plug in transparently — the default just wraps the passed provider 1:1.
- Hooks are dispatched **directly inside the loop**, not as a side layer: `dispatchSessionStart`/`dispatchSessionEnd` (`loop.go:1942`, `1963`) and `dispatchBeforeTool`/`dispatchAfterTool` (`loop.go:1895`, `1918`) wrap the turn and each tool call; `blockedByHookResult` (`loop.go:1988`) can short-circuit a tool call the hook rejects.
- `executeToolCall` (`loop.go:1135`, ~400 lines) is the single choke point for running a tool: decodes arguments, consults `ArgsPermissioner`/`PrePermissionRejecter`, runs sandbox-permission logic (`sandboxRequest`, `loop.go:2184`), retries with escalated sandbox/network permissions on denial (`maybeRetryUnsandboxedAfterSandboxRestriction`, `maybeRetryWithNetworkAfterSandboxDenial`), and only then calls `tool.Run(ctx, args)`.
- Context management is handled by a `contextPlanner` (`newContextPlanner`, referenced around `loop.go:190`) that composes the system prompt from `promptParts` and tracks a `contextWindow`/`promptCacheKey`/`serviceTier` — i.e. context-budget planning is a first-class concern inside the loop, not an afterthought.

## What's reusable vs. worth diverging from for liam

**Reusable patterns (either architecture):**
- A small `Tool` interface (`Name`, `Description`, `Parameters`/schema, `Run`) is the common shape; liam's own `Provider`-interface decision (already locked) pairs naturally with a hand-rolled `Tool` interface like zero's rather than pulling in a third-party agent framework like fantasy — stdlib-first preference favors zero's approach.
- Map + constructor-function registration (kit) is simpler than zero's `registry.go` object for a "basic toolset, keep default context small" v1 — worth checking zero's `Registry` shape before deciding, but kit's flat map is the lower-ceremony option.
- zero's `Safety`/`Permission` fields living on the Tool interface itself (not bolted onto the loop) directly informs ticket #22 (permission/approval model) — recommend surfacing this precedent there.
- zero's `Display`/`ToolOutcome` split (model-facing text vs. TUI-facing preview, zero token cost for the preview) is directly relevant to the "keep default context small" fog item and to the TUI-shell prototype ticket (#23).
- Both defer MCP tool loading so startup isn't blocked (kit explicitly; zero less clear from this pass) — worth carrying into liam's MCP-scope ticket (#15).

**Worth diverging from:**
- kit's dependency on `charm.land/fantasy` for the agent loop itself conflicts with liam's "strongly prefer stdlib, exceptions where it makes sense" — fantasy is a reasonable *exception candidate* only if it substantially simplifies streaming/provider abstraction; otherwise zero's hand-rolled `Run()` is closer to the stdlib-preferring spirit, at the cost of far more code to own (3487 lines in one file — likely more than liam wants for a *minimal* harness).
- Neither tool interface generates JSON schema from Go struct tags/reflection — both hand-write schema maps/structs. liam could still choose to generate schema from struct tags (a lighter stdlib-only exception, e.g. via `encoding/json` struct tags plus a small reflection helper) to cut boilerplate, if that fits "minimal."
- zero's `executeToolCall` bundles sandbox-permission retry logic tightly into the loop; liam's decoupled headless/TUI requirement (already locked) argues for keeping permission/sandbox logic in a separate seam the loop calls into, rather than inlining ~400 lines like zero does.
