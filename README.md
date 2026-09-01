# liam

A minimal coding-agent harness written in idiomatic Go, run as a terminal
program — interactive TUI or headless. Strongly prefers the Go standard
library, ships a small coding-focused built-in toolset, and supports Hooks,
MCP, and Agent Skills through a small number of well-chosen seams.

## Features

- **Interactive TUI or headless** — a Bubbletea shell with auto-detected
  light/dark (Catppuccin Latte/Frappe) theming, or `liam -p "<prompt>"` for
  scripting/CI.
- **OpenRouter, auto-routing by default** — `openrouter/auto` picks the
  model per turn; any other OpenRouter model string works too.
- **Built-in toolset**: `read`, `write`, `edit`, `bash`, `find`, `grep`,
  `web_fetch`, and `web_search` (when `EXA_API_KEY` is set).
- **MCP client** (stdio) — connect any MCP server via `mcpServers` config,
  with an optional per-server tool allow-list.
- **Agent Skills** (agentskills.io) — model-driven progressive disclosure,
  project skills are trust-gated with a one-time prompt.
- **Hooks** — `sessionStart`/`sessionEnd`/`beforeTool`/`afterTool`, each an
  external command that can allow/deny (`beforeTool`) or just observe.
- **Project instructions** — `AGENTS.md`/`LIAM.md`, discovered walking from
  the git root down to your working directory, plus a personal
  `$XDG_CONFIG_HOME/liam/LIAM.md`.
- **Context management** — automatic compaction as a session approaches its
  context limit, plus live context/cost tracking in the status line.
- **Customizable status line** — the built-in one, or point `statusLine` at
  your own external command.
- **No built-in permission system** — liam runs tools with the harness
  process's own OS permissions; if you want gating (confirm-before-shell,
  deny-by-default, etc.), build it as a `beforeTool` Hook.

## Install

```sh
go install github.com/mgoodness/liam/cmd/liam@latest
```

Or grab a prebuilt binary (darwin/linux, amd64/arm64) from the
[releases page](https://github.com/mgoodness/liam/releases).

## Usage

```sh
export OPENROUTER_API_KEY=...
liam                        # interactive TUI
liam -p "your prompt here"  # headless: one prompt, then exit
```

Optional environment variable:

```sh
export EXA_API_KEY=...  # enables the web_search tool; unset = tool isn't registered
```

Flags:

| Flag       | Description                                                                     |
| ---------- | -------------------------------------------------------------------------------- |
| `-p`       | Send a single prompt headlessly and exit, instead of opening the TUI.            |
| `-model`   | Override `provider.model` for this run.                                          |
| `-skill`   | Force-activate a skill by name before the prompt (headless mode only, requires `-p`). |
| `-version` | Print the version and exit.                                                      |

Interactive-mode slash commands: `/quit`, `/clear` (reset the session), `/skills` (list discovered skills), `/<skill-name>` (force-activate a skill by name, bypassing model judgment).

## Configuration

`liam.jsonc` (JSONC — comments allowed), loaded from
`$XDG_CONFIG_HOME/liam/liam.jsonc` (global) and `liam.jsonc` walked up from
your working directory (project), deep-merged global-then-project, with
`LIAM_*` environment variables and CLI flags layered on top:

```jsonc
{
  "provider": { "model": "openrouter/auto" },
  "theme": { "mode": "auto" }, // "auto" | "dark" | "light"
  "hooks": {
    "beforeTool": [{ "command": "my-policy-check.sh", "match": ["bash"] }],
  },
  "mcpServers": {
    "my-server": {
      "command": "my-mcp-server",
      "args": ["--stdio"],
      "env": { "API_KEY": "$MY_API_KEY" },
      "tools": ["some_tool"], // omit to register every tool the server exposes
    },
  },
  "skills": {
    "paths": ["/extra/skills/dir"],
    "disabled": ["some-skill-name"],
    "trustProjectSkills": true, // skip the one-time project-skills trust prompt
  },
  "statusLine": {
    "command": "my-statusline.sh", // omit for the built-in status line
    "refreshInterval": 1000,
  },
}
```

Skills are discovered from `.agents/skills/` and `.liam/skills/` at both
project scope (walked from the git root down) and user scope (`~/.agents/skills/`,
`$XDG_CONFIG_HOME/liam/skills/`), plus any extra `skills.paths`.

## Design notes

liam ships no built-in permission/confirmation system, no sandbox, and no
`--yolo` flag — tools run with your own OS permissions, same as running them
yourself in a shell. See [`docs/adr/0004-no-built-in-permission-system.md`](docs/adr/0004-no-built-in-permission-system.md)
for the reasoning, and [`CONTEXT.md`](CONTEXT.md) for the project's domain
vocabulary. [`CHANGELOG.md`](CHANGELOG.md) tracks releases.
