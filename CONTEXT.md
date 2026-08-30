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

**Plugin**:
An installable, distributable bundle (per the agent-plugins.org spec) that extends the harness itself by registering new tools, skills, hooks, or MCP servers together as one unit.
_Avoid_: skill, extension, addon.

**MCP server**:
An external process the harness connects to via the Model Context Protocol, exposing its own tools and resources to the model from outside the harness's compiled-in toolset.
_Avoid_: plugin, integration.

**Provider**:
A backend that serves model completions. OpenRouter is the default provider; its auto-routing mode picks the underlying model per request.
_Avoid_: model (a provider serves many models), backend.

**Session**:
One continuous conversation between the user and the harness, with its own history and active toolset, ending when the user exits or explicitly resets it.
_Avoid_: conversation, chat.
