# Liam

A minimal, opinionated coding agent harness, in the spirit of Mario Zechner's [pi](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/), built on Go's standard library plus a small number of dependencies.

## Language

**Provider**:
The abstraction over a single LLM backend — handles authentication and turns a message history into a model response. v0.1 has exactly one implementation, Copilot Chat.
_Avoid_: Model, backend, client (too generic — those mean the underlying LLM or its SDK, not this abstraction).

**Tool**:
A capability the model can invoke as part of a response — one of `read`, `write`, `edit`, or `bash` in v0.1. Distinct from the Go `tool` package that implements them.
_Avoid_: Function, action.

**Agent loop**:
The core control flow: send the message history to the Provider, execute any Tool calls in the response, feed the results back, repeat until a response contains no Tool calls.
_Avoid_: Orchestrator, pipeline.

**YOLO mode**:
The v0.1 safety stance: Tool calls execute immediately with no confirmation prompt and no sandboxing. Adopted from pi's position that permission checks are theater once arbitrary code execution is already allowed.
_Avoid_: Unsafe mode, unrestricted mode.

**GitHub OAuth token vs. Copilot token**:
Two distinct credentials in the auth flow. The *GitHub OAuth token* is obtained once via device-flow login and identifies the user to GitHub. The *Copilot token* is a short-lived token exchanged from the GitHub OAuth token, and is what's actually sent to `api.githubcopilot.com` for chat requests. Both are persisted under the XDG config file; the Copilot token is re-exchanged when it expires.
_Avoid_: Access token (ambiguous between the two), API key.
