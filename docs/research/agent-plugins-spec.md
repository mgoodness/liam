# agent-plugins.org Plugin spec (v1.0.0)

Sources: [agent-plugins.org](https://agent-plugins.org), [agent-plugins.org/specification](https://agent-plugins.org/specification), spec repo [github.com/agentplugins/agent-plugins-spec](https://github.com/agentplugins/agent-plugins-spec).

## What it is

"An open, vendor-neutral standard for packaging reusable components into portable plugins," meant to solve fragmentation across incompatible per-client plugin formats. It defines a **portable container format**, not a registry or install mechanism.

## Bundle format

A plugin is a directory with a required manifest and two fixed component locations:

```
my-plugin/
├── plugin.json           # required manifest
├── skills/                # fixed: Agent Skills (agentskills.io spec)
│   └── summarize/
│       ├── SKILL.md
│       ├── scripts/
│       └── references/
├── mcp.json               # fixed: MCP server declarations
└── com.example.client/    # client-specific extension namespace (reverse-domain)
    └── hooks/
```

`plugin.json` required field: `$schema` (must be `https://agent-plugins.org/schemas/1.0.0/plugin.schema.json`) and `name` (1–64 chars, lowercase alphanumeric/hyphen/period, no leading/trailing or consecutive separators). Optional: `version` (semver recommended), `description`, `author` (`name`/`email`/`url`), `homepage`, `repository`, `license` (SPDX), `keywords`, `extensions` (reverse-domain namespaced, client-specific data). Schema is **closed**: only those top-level fields are permitted; unknown fields must be reported and ignored, not rejected.

All plugin-relative paths must start with `./`, resolve against the plugin root, and stay within it after resolution — a containment requirement against path escape.

## Component registration

**Skills** — discovered from `skills/`. Each immediate child directory with a `SKILL.md` becomes one skill; clients must NOT recurse deeper. Must conform to the [Agent Skills spec](https://agentskills.io/specification). Invalid skills are skipped, not fatal to the whole plugin.

**MCP servers** — declared in `mcp.json` at plugin root:
```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": { "server-name": { "type": "stdio", "command": "...", "args": [], "env": {}, "cwd": "..." } }
}
```
Three variants by `type`: `stdio` (`command`, `args`, `env`, `cwd`), `streamable-http` (`url`, `headers`), `sse` (legacy, same shape as streamable-http). Clients MUST support stdio or streamable-http (SHOULD support both); `sse` support is optional. `mcp.json`'s `$schema` version must match `plugin.json`'s. Invalid server entries are skipped individually.

**Tools and hooks are NOT first-class, portable plugin components in this spec.** There is no fixed `tools/` or `hooks/` directory. Tools reach the model only via MCP servers exposing them (through `mcp.json`) or via skills. Hooks appear only as an *example* of a client-specific extension, placed under a reverse-domain namespace directory (e.g. `com.example.client/hooks/`) — meaning hook support is entirely up to each client's own extension convention, not standardized.

## Environment / path expansion

Clients launching plugin subprocesses must set `PLUGIN_ROOT` (absolute path to the plugin dir) and `PLUGIN_DATA` (absolute path to a client-managed persistent data dir). `${PLUGIN_ROOT}` / `${PLUGIN_DATA}` are expanded (single-pass, non-recursive, exact-match textual replacement) in `mcp.json`'s `args` string elements, `env` values, and `cwd`. The `command` field itself is never expanded — bare names use `$PATH` search, plugin-relative paths resolve directly against plugin root.

## Versioning

Plugin `version` should follow semver (clients may use it for update/staleness checks). Spec versioning covers normative text + both JSON schemas as one unit per release.

## Distribution & installation

**Explicitly out of scope for v1.** The spec only defines the portable container format; it does not define registries, registry APIs, or install mechanisms. "Distribution approaches remain client-defined" — git clone, registry download, local dev path, etc. are all valid so long as the resulting directory structure and containment rules hold.

## Implication for liam

- A liam Plugin loader needs to: read `plugin.json`, validate `$schema`/`name`, walk `skills/*/SKILL.md` (non-recursive), parse `mcp.json` and register declared MCP servers (stdio at minimum, streamable-http recommended).
- Liam itself must define its own installation mechanism (registry, git URL, local path) — the spec gives no guidance here, so this is a liam-specific decision.
- If liam wants plugin-declared **Hooks**, that's a liam-specific extension under a reverse-domain namespace directory (e.g. `dev.liam.cli/hooks/`), not something the spec standardizes — worth flagging to the Hooks-design ticket (#14 in this repo).
- Plugin-declared **tools** (beyond what an MCP server exposes) have no home in this spec; if liam wants native Go tools bundled via a plugin, that's also a liam-specific extension, not portable.
