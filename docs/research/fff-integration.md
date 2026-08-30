# How pi-fff and OpenCode integrate fff for find/grep

Research for [liam#4](https://github.com/mgoodness/liam/issues/4), a child of the [liam wayfinder map](https://github.com/mgoodness/liam/issues/1).

## fff itself

Source: [dmtrKovalenko/fff README](https://github.com/dmtrKovalenko/fff) (fetched via `gh api repos/dmtrKovalenko/fff/readme`).

- Core is Rust (`language: Rust` per the GitHub API), shipped as a prebuilt binary plus native bindings for several hosts (Neovim/Lua, Rust, C, Python, Bun, Node.js).
- Three integration shapes exist:
  1. **MCP server** (`fff-mcp` binary) — exposes `ffgrep`, `fffind`, `fff-multi-grep` as MCP tools. Install via Homebrew (`dmtrKovalenko/fff/fff-mcp`) or a one-line installer script; wired into Claude Code, Codex, OpenCode, Cursor, Cline, etc. Source: `crates/fff-mcp/`.
  2. **pi agent extension** (`@ff-labs/pi-fff`, npm) — see below.
  3. **Neovim plugin** / **native language bindings** (`@ff-labs/fff-bun`, `fff-node`, etc.) — an in-process library, no subprocess/MCP hop. This is what OpenCode uses (see below).

## pi-fff (the pi agent extension)

Source: same README, `<details id="pi-extension">` section; package at `packages/pi-fff/` in the fff repo.

- Install: `pi install npm:@ff-labs/pi-fff`.
- **Tool boundary: two tools**, not one combined tool — `ffgrep` (content search) and `fffind` (path/filename search) — plus it takes over pi's `@`-mention autocomplete.
- **Three runtime modes**, switchable live with `/fff-mode` (or `PI_FFF_MODE` env var / `--fff-mode` flag):
  - `tools-and-ui` (default): adds `ffgrep`/`fffind` as new tools, replaces `@`-mention autocomplete.
  - `tools-only`: adds the tools but keeps pi's native editor autocomplete.
  - `override`: **replaces** pi's built-in `grep`, `find`, and `multi_grep` tools with the FFF implementations outright — i.e. the model still calls the same tool names, but FFF answers.
- `ffgrep` params: `path`, `exclude` (comma/space/array, leading `!` optional), `caseSensitive`, `context`, cursor-based pagination. Auto-detects regex vs. literal, falls back to fuzzy search on zero exact matches, and rejects bare wildcard-only patterns (e.g. `.*`) up front rather than running them.
- `fffind` matches the whole repo-relative path (not just filename), is frecency-aware (recently/frequently opened files rank higher), and flags scattered fuzzy-match noise before it reaches the agent's context window.
- State: frecency/history SQLite-style DBs at `~/.pi/agent/fff/` by default (or existing `fff.nvim` DBs if present), overridable via `FFF_FRECENCY_DB` / `FFF_HISTORY_DB`.

## OpenCode (github.com/anomalyco/opencode, current release v1.18.25 as of this research)

Note: both `github.com/sst/opencode` and `github.com/anomalyco/opencode` currently exist as separate, non-archived repos with an identical description ("The open source coding agent"). Source below is from `anomalyco/opencode`, which is where the fff-integration code and current releases (well past v1.17.0) actually live; the `sst/opencode` repo may be a predecessor/mirror — not resolved further here as it's outside this ticket's question.

Source: `packages/core/src/filesystem/search.ts` in `anomalyco/opencode` (fetched via `gh api repos/anomalyco/opencode/contents/...`).

- **Invocation: in-process library, not subprocess and not MCP.** OpenCode depends on `@ff-labs/fff-bun` (there's also `patches/@ff-labs%2Ffff-bun@0.9.3.patch`, and an `fff.node.ts` variant for non-Bun runtimes) and calls it directly through native bindings (`Fff.create(...)`, `.glob()`, `.grep()`, `.fileSearch()`, `.directorySearch()`, `.mixedSearch()`).
- **Abstraction layer**: a `FileSystemSearch.Service` Effect interface with exactly three operations — `find`, `glob`, `grep` — has two interchangeable implementations:
  - `fffLayer`: backed by fff, created with `{ basePath, aiMode: true, disableMmapCache: true, disableContentIndexing: true }`.
  - `ripgrepLayer`: a ripgrep + `fuzzysort`-based fallback.
  - Selection is automatic: `Flag.OPENCODE_DISABLE_FFF || !Fff.available() ? ripgrepLayer : fffLayer` — fff is the default, ripgrep is the fallback when fff isn't available on the platform or is explicitly disabled via an env flag. If fff itself fails to initialize at runtime, the layer degrades to a service that returns empty results (logged as a warning) rather than crashing.
- **Output is structured, not raw text**, at the `FileSystemSearch.Service` layer: `Entry` objects (`path`, `type: "file" | "directory"`) for find/glob, and `Match` objects for grep (`entry`, `line`, `offset`, `text`, `submatches: [{text, start, end}]`). Grep line text is truncated to 2000 chars with a trailing `...`; `pageSize`/`limit` bound result counts, and grep additionally sets a `timeBudgetMs: 1500` time budget on the native call.
- **Model-facing tool boundary**: separate tools per `packages/opencode/src/tool/` — `grep.ts` and `glob.ts` exist as distinct model tools (no separate model-facing "find" tool was found; fuzzy find appears used internally, e.g. for `@`-mention, rather than exposed as its own tool). Caveat: `grep.ts` as read imports `Ripgrep.Service` directly rather than going through `FileSystemSearch.Service` — so the fff/ripgrep auto-selection shown above may apply to some internal call sites (e.g. the initial repo-wide index used for fuzzy find) without necessarily being what backs every model-facing tool call; this file's actual current wiring wasn't fully traced and would need a closer read if it matters for liam's design.
- At the tool layer (`grep.ts`), output returned to the model is **plain text**, not JSON: a header line (`Found N matches (more matches available)`), then grouped by file path with `Line <n>: <text>` rows, capped at a 100-result limit with a truncation notice appended when hit.

## Takeaways for liam's fff-backed find/grep ticket ([liam#18](https://github.com/mgoodness/liam/issues/18))

- Two viable integration shapes exist for liam, mirroring pi-fff vs. OpenCode: connect to `fff-mcp` as an MCP server (zero custom Go binding code, but adds an external process dependency and the generic MCP client machinery), or link a native fff library binding directly into the Go binary if/when one exists for Go (not confirmed here — only Rust/C/Python/Bun/Node bindings were found in the README; a Go binding was not located and may not exist yet, which would push liam toward the MCP-server path or a CLI-subprocess path instead).
- Both real-world integrations split find and grep into **separate tools** (`ffgrep`/`fffind`, or `grep.ts`/`glob.ts`) rather than one combined tool.
- Whichever path liam takes, keep the model-facing tool output as **plain, human-readable text** (OpenCode's actual tool-response shape) even if the layer underneath returns structured data — matches the "keep default context small" requirement better than dumping raw JSON.
