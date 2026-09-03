# liam

A minimal coding-agent harness written in idiomatic Go, run as a terminal program.

## Language

**Harness**:
The `liam` program itself — the host that runs the agent loop, the TUI, and every extension point (tools, hooks, MCP servers, skills, plugins). This is the artifact this whole effort specs.
_Avoid_: agent, app, runtime, CLI.

**Agent loop**:
The core request/response cycle: send the conversation and tool definitions to the model, execute any tool calls the model requests, feed the results back, repeat until the model stops requesting tools.
_Avoid_: agent (bare), loop, runtime.

**Tool**:
A single function the model can invoke mid-conversation — read a file, run a shell command, search the web — identified by a name and JSON schema, implemented in Go and compiled into the harness.
_Avoid_: capability, action, function.

**Toolset**:
The tools active in a given session: the harness's built-in tools plus whatever any connected MCP servers expose.
_Avoid_: tool list, capabilities.

**Hook**:
A shell command the harness runs synchronously at a defined lifecycle point (e.g. before a tool executes, after a session ends), configured by the user. No model involvement.
_Avoid_: plugin, callback, trigger.

**Skill**:
A packaged, on-demand set of instructions (markdown plus optional scripts, per the agentskills.io spec) that the model can choose to load mid-conversation to change how it approaches a specific kind of task.
_Avoid_: hook, plugin, prompt.

**Slash command**:
User input beginning with `/`: either one of liam's five reserved built-ins (`/quit`, `/clear`, `/compact`, `/skills`, `/theme`) or a bare `/<skill-name>` that force-activates a matching Skill, bypassing model judgment. Only recognized when it's the very start of the input (row 0, column 0) — a `/` elsewhere is ordinary text.
_Avoid_: command (bare — could be confused with a tool call or CLI flag).

**Transcript**:
The scrolling, read-only log of the session's turns — every user and assistant message, tool call, and system notice rendered so far, occupying the fixed-height region above the input. Mouse-wheel scrolling and click-drag text selection both act on the transcript specifically, not the input or status block.
_Avoid_: scrollback, conversation view, history (bare — could be confused with input-history recall).

**Popup**:
A transient, fuzzy-matched suggestion list that opens as the user types a specific trigger character, floats as a bordered dialog above the input while active, and closes on selection, Esc, or the trigger token breaking — liam has two: the `@`-mention (file reference) popup and the `/`-command (Slash command) popup. Distinct from the status block, which is always-present rather than transient.
_Avoid_: modal (describes its rendering, not the concept — a popup floats as a dialog, but "modal" alone doesn't capture the autocomplete/suggestion behavior), dropdown, autocomplete (bare — describes what it does, not the harness-level concept).

**Banner**:
The three-line identity block (name/version, provider paired with model, working directory) shown as the very first entry in the transcript at the start of every session, including after `/clear` — scrolls away with the rest of the transcript like any other line, never pinned to the screen. No logo or wordmark accompanies it; a few visual treatments were tried and dropped as not worth the complexity.
_Avoid_: header, splash screen, welcome message.

**Plugin**:
An installable, distributable bundle (per the agent-plugins.org spec) that extends the harness itself by registering new tools, skills, hooks, or MCP servers together as one unit.
_Avoid_: skill, extension, addon.

**MCP server**:
An external process the harness connects to via the Model Context Protocol, exposing its own tools and resources to the model from outside the harness's compiled-in toolset.
_Avoid_: plugin, integration.

**Provider**:
A backend that serves model completions. OpenRouter is the default provider; its auto-routing mode picks the underlying model per request.
_Avoid_: model (a provider serves many models), backend.

**Credential**:
Long-lived, provider-specific authentication material liam persists across sessions — an OAuth refresh token, not a static secret — because the Provider it belongs to requires an interactive login rather than an exportable API key. Distinct from every other external-service key (Exa, OpenRouter), which liam reads from its own environment variable and never persists itself.
_Avoid_: API key (a Credential is specifically the persisted-OAuth-token case), token (bare — ambiguous with e.g. a tool-call ID).

**Session**:
One continuous conversation between the user and the harness, with its own history and active toolset, ending when the user exits or explicitly resets it. Ending doesn't mean losing it: a Session persists to disk and can be resumed. Not to be confused with a **Subagent**'s internal conversation, which has no user-driven lifecycle at all and is never persisted.
_Avoid_: conversation, chat.

**Subagent**:
A nested instance of the agent loop, spawned in-process by the model (via a dedicated tool) to delegate a bounded sub-task, sharing the parent Session's toolset and working directory but starting with a fresh, independent context budget. Completes automatically when its delegated task returns — it has no user-facing lifecycle, and is not itself a Session.
_Avoid_: session (bare — see Session's own entry), sub-session, agent (bare).

**Compaction**:
Condensing a session's older turns into a single model-generated summary once a sliding window of recent turns is exceeded, so a long conversation doesn't hit the model's context limit. Triggered manually (`/compact`) or automatically at ~85% context usage, or reactively after a `ContextTooLong` provider error.
_Avoid_: truncation, pruning, summarization (bare — compaction is the harness-level mechanism; summarization is the model call it makes internally).

**Identity preamble**:
A fixed, non-configurable block of text — liam's name and a one-line description of what it does — that the harness always prepends to a turn's assembled system prompt, ahead of any discovered project instructions. Establishes what liam is regardless of the underlying model or whether a project defines its own `AGENTS.md`/`LIAM.md`.
_Avoid_: system prompt (bare — the full assembled system prompt includes this plus project instructions), preamble (bare).
