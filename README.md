# liam

A minimal, opinionated coding agent harness, in the spirit of Mario Zechner's [pi](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/) — a small system prompt, four tools, no framework, and a hand-written agent loop, running on GitHub Copilot instead of a separate API key. See [`docs/references.md`](docs/references.md) for the full set of design influences and [`CONTEXT.md`](CONTEXT.md) for the project's vocabulary.

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

From there it's a REPL: type a message, leave a blank line to submit it (so pasting multi-line content like a stack trace works as one turn), and liam drives the agent loop — calling Copilot, running any tool calls it requests, and feeding the results back — until it has a final answer.

- **Exit**: Ctrl+D, Ctrl+C, or a literal `/exit` / `/quit` line.
- **Model override**: `./liam --model <id>` to use something other than the default.

### Tools

liam gives the model four tools, deliberately not more:

| Tool | What it does |
| --- | --- |
| `read` | Read a file's contents, with an offset/limit for large files |
| `write` | Create or overwrite a file, auto-creating missing parent directories |
| `edit` | Replace an exact, uniquely-matching span of text in a file |
| `bash` | Run a shell command, with a default timeout the model can override per call |

The model reaches for `bash` for anything not covered above — there's no dedicated `grep`/`find`/`ls`.

## How it's built

Four packages, each with a narrow job:

- **`provider`** — the `Provider` interface plus the Copilot implementation: device-flow login, credential storage/refresh, and turning a message history into a response (final text or tool calls).
- **`tool`** — the four tools' definitions and handlers behind a fixed dispatch table.
- **`agent`** — the loop tying a `Provider` and the tool dispatch table together: call the provider, run any tool calls, feed results back, repeat until a response has none.
- **`cmd/liam`** — the REPL and CLI wiring that make it a runnable program.

v1 has no third-party Go dependencies — `net/http`, `encoding/json`, `os/exec`, and `context` cover everything it needs.

## Status

v1 is feature-complete against its [original spec](https://github.com/mgoodness/liam/issues/1): device-flow auth, the four tools, and an interactive REPL, all backed by Copilot Chat. Deliberately out of scope for now: session save/resume, hierarchical `AGENTS.md` project context, custom slash commands, streaming responses, and any permission/confirmation layer (YOLO mode is a final decision, not a placeholder). See the spec issue and [`docs/adr/`](docs/adr/) for the reasoning behind what's here and what isn't.

## License

GPL-3.0 — see [`LICENSE`](LICENSE).
