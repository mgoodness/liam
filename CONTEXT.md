# Liam

A minimal, opinionated coding agent harness, in the spirit of Mario Zechner's [pi](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/), built on Go's standard library plus a small number of dependencies.

## Language

**Provider**:
The abstraction over a single LLM backend — handles authentication and turns a message history into a model response. v0.1 has exactly one implementation, Copilot Chat.
_Avoid_: Model, backend, client (too generic — those mean the underlying LLM or its SDK, not this abstraction).

**Tool**:
A capability the model can invoke as part of a response — `read`, `write`, `edit`, and `bash` in v0.1; `web_search` and `web_fetch` added in v0.2. Distinct from the Go `tool` package that implements them.
_Avoid_: Function, action.

**`web_search` vs. `web_fetch`** (v0.2):
Two distinct Tools, not one with modes. `web_search` queries the Brave Search API and is only present in the tool set when a Brave API key is configured in the environment — silently absent otherwise, never an error. `web_fetch` retrieves a specific, already-known URL directly via HTTP and needs no API key at all, since it isn't going through a third-party indexing service; it's always available. `web_fetch` converts HTML to readable text and relies on the existing tool-output truncation cap rather than a secondary summarization pass — see ADR 0004.
_Avoid_: Search tool (ambiguous between the two), browsing.

**Agent loop**:
The core control flow: send the message history to the Provider, execute any Tool calls in the response, feed the results back, repeat until a response contains no Tool calls. From v0.2, the loop reports its own progress via a `Callbacks` struct (assistant text, each Tool call, each Tool result) invoked synchronously as it happens — purely observational, not a permission gate; see ADR 0006. `YOLO mode` is unaffected.
_Avoid_: Orchestrator, pipeline.

**YOLO mode**:
The v0.1 safety stance: Tool calls execute immediately with no confirmation prompt and no sandboxing. Adopted from pi's position that permission checks are theater once arbitrary code execution is already allowed.
_Avoid_: Unsafe mode, unrestricted mode.

**GitHub OAuth token vs. Copilot token**:
Two distinct credentials in the auth flow. The *GitHub OAuth token* is obtained once via device-flow login and identifies the user to GitHub. The *Copilot token* is a short-lived token exchanged from the GitHub OAuth token, and is what's actually sent to `api.githubcopilot.com` for chat requests. Both are persisted under the XDG config file; the Copilot token is re-exchanged when it expires.
_Avoid_: Access token (ambiguous between the two), API key.

**AGENTS.md discovery** (v0.2):
The two-level process that builds liam's system-prompt context: a global file (`$XDG_CONFIG_HOME/liam/AGENTS.md`) loaded first, then every `AGENTS.md` found walking from the current directory up to the git repository root (or filesystem root if not in a repo), concatenated in root-to-cwd order with no labels distinguishing them. Missing files are skipped silently; a read error warns to stderr and skips that file. Matches pi's own discovery mechanism exactly.
_Avoid_: Project context, config discovery.

**Skill** (v0.2):
A `skill-name/SKILL.md` directory conforming to the open [Agent Skills specification](https://agentskills.io/specification), discovered from three vendor-neutral locations, none namespaced to liam itself (see ADR 0008): two global — `$XDG_CONFIG_HOME/agents/skills/` (falling back to `~/.config/agents/skills/`) and `~/.agents/skills/`, the convention actually in active use across the wider agent-skills ecosystem, checked regardless of `XDG_CONFIG_HOME` — plus one project directory, `.agents/skills/` at the git repository root. A same-named Skill in both global locations resolves in favor of `~/.agents/skills/`; a project Skill in turn overrides a same-named global one, the same override rule as AGENTS.md's "closest wins" convention. `name` must be lowercase alphanumeric+hyphens, ≤64 characters, no leading/trailing/consecutive hyphens, and must match its parent directory name; a Skill failing validation is skipped with a warning, not a startup failure. A Skill's `name` is also its `/name` slash command (liam's own additive extension, not part of the open spec — see ADR 0003); its `description` additionally makes it eligible for model-invocation via a lightweight name+description index, unless `disable-model-invocation` is set (a Claude-Code-originated convention honored for compatibility, though not spec-mandated). Optional bundled `scripts/`/`references/`/`assets/` directories need no special handling — the model reaches them via the `read`/`bash` Tools once it knows the path. Loading a Skill's own full content likewise reuses `read` rather than a dedicated mechanism.
_Avoid_: Command, template, extension (pi's own term for its code-based, non-Skill mechanism — a distinct concept liam doesn't have).
