# Skills and MCP config never read another tool's directories directly

liam never reads another coding agent's live config or state (Claude Code's `.claude/skills/`, `~/.claude.json`/`.mcp.json`, etc.) as a first-class source. Interop with skills or MCP servers already set up for another tool goes through liam's own conventions instead — its own Skills discovery paths (deliberately excluding `.claude/skills/`, per [issue #41](https://github.com/mgoodness/liam/issues/41)) and its own `mcpServers` config — even where reading the other tool's files directly would be more convenient. This keeps liam's config/discovery surface fully self-contained and predictable, at the cost of the user needing to duplicate or reference material rather than getting free interop.

## Considered Options

jcode reads Claude Code's own `~/.claude.json`/`.mcp.json` live on every load as a first-class MCP config source, with no import step ([docs/research/pi-go-jcode-prior-art.md](../research/pi-go-jcode-prior-art.md)). Rejected for MCP config for the same reason liam already rejected it for Skills discovery: reading another tool's live files couples liam's behavior to that tool's file layout and versioning, and a config change in the other tool would silently change liam's behavior with no liam-side edit to explain it.
