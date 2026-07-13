# liam

A minimal, opinionated coding agent harness, in the spirit of Mario Zechner's [pi](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/) — a small system prompt, a hand-written agent loop, and no framework, running on GitHub Copilot instead of a separate API key. See [`docs/references.md`](docs/references.md) for the full set of design influences and [`CONTEXT.md`](CONTEXT.md) for the project's vocabulary.

## ⚠️ YOLO mode

liam runs in **YOLO mode**: tool calls execute immediately, with no confirmation prompt and no sandboxing. Once you start it, the model can read, write, and edit any file it can reach, and run any shell command, without asking first. Run it somewhere you're comfortable giving that level of access — a container or a disposable checkout, not your only copy of anything important.

## Install

Requires Go 1.26+ and an active GitHub Copilot subscription.

```sh
go build -o liam ./cmd/liam
```

## Usage

```sh
./liam
```

On first run, liam walks you through a device-flow login: it prints a one-time code and a URL, you authorize it in your browser, and it persists the resulting credentials to `$XDG_CONFIG_HOME/liam/credentials.json` (falling back to `~/.config/liam/`) so you won't need to log in again.

From there it's a REPL: type a message and press Enter to submit it, and liam drives the agent loop — printing the model's intermediate text and a one-line summary of each tool call as they happen, calling Copilot, running any tool calls it requests, and feeding the results back — until it has a final answer.

- **Multi-line input**: Shift+Enter (or the universal `Ctrl+J` fallback) inserts a newline without submitting; pasting multi-line content (e.g. a stack trace) always arrives and submits as one message, regardless of a trailing newline.
- **Interrupt**: Ctrl+C cancels the current turn in flight (a hung `bash` call, a slow model response) and returns you to the prompt; pressed again while idle at the prompt, it ends the session.
- **Exit**: Ctrl+D on an empty input line, or a literal `/exit` / `/quit` line. Ctrl+D with partial typed input submits it instead, the same as Enter.
- **Model override**: `./liam --model <id>` to use something other than the default.

### Project context

liam reads a global `AGENTS.md` (`$XDG_CONFIG_HOME/liam/AGENTS.md`, falling back to `~/.config/liam/AGENTS.md`), then every `AGENTS.md` found walking from your current directory up to the git repository root, folding them all into its system prompt — the same discovery convention as pi and other coding agents.

### Skills

Drop a `skill-name/SKILL.md` directory (conforming to the open [Agent Skills specification](https://agentskills.io/specification)) into `$XDG_CONFIG_HOME/liam/skills/` (global) or `.liam/skills/` (project) and liam picks it up automatically: its name and description are indexed into the system prompt so the model can decide to use it on its own, or you can invoke it explicitly by typing `/skill-name`.

### Tools

liam gives the model six tools:

| Tool | What it does |
| --- | --- |
| `read` | Read a file's contents, with an offset/limit for large files |
| `write` | Create or overwrite a file, auto-creating missing parent directories |
| `edit` | Replace an exact, uniquely-matching span of text in a file |
| `bash` | Run a shell command, with a default timeout the model can override per call |
| `web_fetch` | Retrieve a URL's content, converted from HTML to readable text |
| `web_search` | Search the web via the Brave Search API |

The model reaches for `bash` for anything not covered above — there's no dedicated `grep`/`find`/`ls`. `web_search` requires a Brave Search API key (`BRAVE_API_KEY` in the environment); without one, it's simply absent from the tool set rather than present-but-erroring. `web_fetch` needs no API key and is always available.

## How it's built

Five packages, each with a narrow job:

- **`provider`** — the `Provider` interface plus the Copilot implementation: device-flow login, credential storage/refresh, and turning a message history into a response (final text or tool calls).
- **`tool`** — the tools' definitions and handlers, assembled per-run into a dispatch table (so `web_search`'s presence can depend on whether an API key is configured).
- **`agent`** — the loop tying a `Provider` and the tool dispatch table together: call the provider, run any tool calls, feed results back, repeat until a response has none. Reports its own progress via a `Callbacks` struct as it goes, and returns promptly if its context is cancelled mid-turn.
- **`skill`** — discovers, validates, and indexes `SKILL.md` directories from the global and project skill locations.
- **`cmd/liam`** — the REPL and CLI wiring that make it a runnable program: raw-mode terminal input, `AGENTS.md` discovery, and slash-command resolution.

liam has a small number of third-party Go dependencies (`golang.org/x/net`, `golang.org/x/term`, `charmbracelet/x/ansi`, `charmbracelet/x/input`, `gopkg.in/yaml.v3`) alongside the standard library — never a zero-dependency requirement, just a small one.

## Status

v0.1 and v0.2 are both shipped: device-flow auth, the core tool set plus web tools, an interactive REPL with raw-mode input (immediate-submit Enter, Shift+Enter/`Ctrl+J` for newlines, reliable multi-line paste), hierarchical `AGENTS.md` project context, Skills, and agent-loop progress reporting, all backed by Copilot Chat. Deliberately out of scope for now: session save/resume, custom (non-Skill) slash commands, streaming responses, and any permission/confirmation layer (YOLO mode is a final decision, not a placeholder). See the [v0.1](https://github.com/mgoodness/liam/issues/1) and [v0.2](https://github.com/mgoodness/liam/issues/14) specs and [`docs/adr/`](docs/adr/) for the reasoning behind what's here and what isn't.

## License

GPL-3.0 — see [`LICENSE`](LICENSE).
