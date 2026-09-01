# Is there a Go library that works like fff?

Standalone research note. Prompted by [liam#97](https://github.com/mgoodness/liam/issues/97), which
questions whether MCP-to-`fff-mcp` (see `internal/mcp/fff.go`) is the right long-term architecture
for liam's `find`/`grep` tools — this note answers the underlying factual question ("does a Go-native
or Go-callable equivalent exist?") but does not act on #97 or any other ticket.

Cross-reference: [`fff-integration.md`](./fff-integration.md) already covers how `fff` itself is
integrated by pi-fff (MCP) and OpenCode (native Bun/Node bindings), and states a Go binding "was not
located and may not exist yet." This note re-verifies that claim more exhaustively and, separately,
surveys Go-native libraries that cover the same *category* of functionality (fuzzy find + frecency +
git-awareness + content grep) rather than fff itself. Findings below don't repeat that file's content.

Comparison rubric (per `fff-integration.md`'s description of fff's actual capabilities):
1. Fast fuzzy file/path finding across a large repo, frecency-ranked.
2. Git-aware (boosts/tags dirty or recently-changed files).
3. Content grep with regex support, results grouped by file.
4. Usable both as a human CLI and as a long-lived, programmatic AI-agent/MCP backend.

All primary-source checks below were performed 2026-09-01 via `gh api`, raw GitHub source fetches, and
pkg.go.dev, unless noted otherwise as secondhand.

## 1. Direct Go bindings for fff itself

Re-checked the `dmtrKovalenko/fff` repo tree directly (`gh api repos/dmtrKovalenko/fff/contents/crates`
and `.../packages`, 2026-09-01):

- `crates/`: `fff-c`, `fff-core`, `fff-grep`, `fff-mcp`, `fff-nvim`, `fff-python`, `fff-query-parser` — no Go crate.
- `packages/`: `fff-bin-*` (per-platform prebuilt binaries), `fff-bun`, `fff-node`, `fff-python`, `pi-fff`, `shared` — no Go package.

The README (`README.md`, `dmtrKovalenko/fff`) does explicitly acknowledge a Go path is *possible*:

> Stable C ABI. Bind from C/C++, Zig, Go via cgo, Python via ctypes, or anything with C FFI.

That line sits in the C-bindings section (`crates/fff-c/`, header at
[`crates/fff-c/include/fff.h`](https://github.com/dmtrKovalenko/fff/blob/main/crates/fff-c/include/fff.h),
built as `libfff_c.{so,dylib,dll}`). So a Go binding via `cgo` against `libfff_c` is architecturally
open — but nobody has built and published one:

- pkg.go.dev search for `fff` + `frecency` returns **zero** matching modules.
- `gh search code "dmtrKovalenko/fff"` (looking for the string anywhere in a `go.mod`, source comment,
  etc.) returns only blog/aggregator hits (awesome-lists, trending-repo snapshots) — no actual Go
  project referencing it.
- `gh search repos` for `"fff go binding"` / `"fff-go"` returns unrelated repos (a GraphQL demo, unit
  test scaffolds, an unrelated archived Go Slack bot) — nothing that binds `dmtrKovalenko/fff`.

**Conclusion: unchanged from `fff-integration.md`, but now confirmed by exhaustive search rather than a
single README read** — no Go binding for fff exists today, official or third-party. The only route is
writing one from scratch via `cgo` + `libfff_c` (real work, not "link an existing library") — see §5 for
what that would actually involve and cost, scoped to liam's real target platforms.

## 2. Fuzzy file finding with frecency (Go-native candidates)

None of the following replicate fff's *combination* of fuzzy-match + frecency + git-awareness; each
covers at most one piece.

| Library | Verified state | What it actually does |
|---|---|---|
| [`sahilm/fuzzy`](https://github.com/sahilm/fuzzy) | Real, active — Go, 1,446 stars, last push 2026-06-24 | Sublime-Text-style fuzzy string/filename matcher over a caller-supplied `Source`. No frecency, no git. Notably it's the matcher **`charmbracelet/bubbles`** uses: `list/list.go` imports `github.com/sahilm/fuzzy` and calls `fuzzy.Find`/`fuzzy.FindNoSort` in `DefaultFilter`/`UnsortedFilter` (confirmed via raw source, lines ~17, 96, 99, 111, 114). |
| [`lithammer/fuzzysearch`](https://github.com/lithammer/fuzzysearch) | Real, active — Go, 1,324 stars, last push 2026-09-01 (today) | Match + Levenshtein-distance ranking (`Match`, `RankMatch`, `Find`, `RankFind`). No frecency, no git. |
| [`ktr0731/go-fuzzyfinder`](https://github.com/ktr0731/go-fuzzyfinder) | Real, active — Go, 518 stars, last push 2026-08-24 | "fzf-like fuzzy-finder as a Go library" — but it's an **interactive TUI widget** (built on `tcell`), not a headless function returning a ranked list for a programmatic caller. Doesn't depend on fzf's own algorithm (`go.mod` shows only `tcell`/`termbox`/etc., no `junegunn/fzf`). Wrong shape for an agent tool backend. |
| [`junegunn/fzf`](https://github.com/junegunn/fzf) | Real, very active — **Go**, 82,767 stars, last push 2026-08-31 | Unlike fff, fzf's core is genuinely Go. Its match algorithm lives in an actual importable, non-`main` package: `src/algo/algo.go` starts `package algo` (confirmed via raw fetch) — so `import "github.com/junegunn/fzf/src/algo"` is literally usable. This is the closest real-world match to the research prompt's "fzf's algorithm as a library" idea. Caveats: it's an internal package of a CLI-first project, not a published/versioned SDK API, and fzf's frecency/history support (`--history`) is a CLI/session feature, not exposed as a library API alongside `algo`. |
| [`charlievieth/fastwalk`](https://github.com/charlievieth/fastwalk) | Real, active — Go, 144 stars, last push 2026-08-20 | Fast parallel directory traversal; is itself a dependency in fzf's own `go.mod`. Plausible building block for the "fast walk" half of a hand-rolled find (analogous role to fff's Rust-side `ignore`/`zlob` walker), but no fuzzy/frecency/git logic of its own. |
| [`mattn/go-zglob`](https://github.com/mattn/go-zglob) | Real, active-ish — Go, 201 stars, last push 2026-04-13 | Glob matching with `**` support. Pure glob, no fuzzy/frecency. |

### Frecency specifically

- No standalone, widely-used Go frecency package exists on pkg.go.dev (searched directly, zero results).
- [`common-fate/granted`](https://github.com/common-fate/granted) — real, active Go CLI for AWS access
  (1,765 stars, last push 2026-08-31) — ships a small internal frecency scorer at
  `pkg/frecency/frecency.go` (fetched directly): a `FrecencyStore`/`Entry` type with
  `FrequencyWeight`/`DateWeight`-tunable scoring and a JSON-persisted cache, exposed via
  `Load`/`GetFrecentEntriess`. It's used to rank **AWS profile selections**, not files — the algorithm
  (~100 lines) is generic enough to adapt but is not published as a standalone reusable module (it
  imports `github.com/fwdcloudsec/granted/pkg/config`, a different module path than the repo it's
  fetched from — evidence of a rename/fork history, not investigated further here). Worth reading as a
  reference implementation, not worth depending on directly.
- `mixmaxhq/frecency` and `johnsylvain/frecent` (surfaced via web search) are JavaScript/TypeScript, not
  Go — excluded on language grounds without further review.

## 3. Content grep as a Go library

No Go equivalent to ripgrep exists as a genuinely importable **library**. Two Go CLI tools sit in the
same space, both checked directly:

- [`monochromegane/the_platinum_searcher`](https://github.com/monochromegane/the_platinum_searcher)
  ("pt", an ack/ag-alike) — 2,818 stars but **last pushed 2023-08-01**, i.e. stale for ~3 years as of
  this research. Abandoned; not a real candidate.
- [`svent/sift`](https://github.com/svent/sift) ("a fast and powerful alternative to grep") — real and
  actively maintained (1,651 stars, last push 2026-06-30). However its source
  (`sift.go`, `matching.go`, `output.go`, etc., checked directly) is entirely `package main` — a
  monolithic CLI binary, not factored into an importable package. Using it means shelling out to the
  compiled binary (the same subprocess pattern liam already uses for `fff-mcp`/ripgrep), not adding it
  as a Go module dependency.

liam's existing stdlib fallback (`filepath.WalkDir` + `regexp`, per `internal/mcp/fff.go`'s fallback
path) is, per this research, roughly on par with what the Go ecosystem otherwise offers *as a library*
for content grep. Nothing found closes that gap without either shelling out to an external grep binary
or hand-rolling further.

## 4. Any single Go library covering find + grep + frecency + git-awareness together?

None found. Every search path (pkg.go.dev, `gh search code`/`repos`, general web search combining
"frecency" + "git-aware" + Go) surfaced either non-Go prior art (fff itself, `mixmaxhq/frecency`,
zoxide-style tools) or narrow single-purpose Go pieces — a fuzzy matcher, *or* a frecency store, *or* a
grep CLI, *or* a git-status library — never bundled the way fff bundles them behind one resident-process
API (`FileFinder.create()` once, then warm-memory queries — README, "What is FFF and why use it over
ripgrep or fzf?" section).

To approximate fff's four capabilities in Go today, liam would need to compose at minimum:

1. A fuzzy matcher — `sahilm/fuzzy` or fzf's `src/algo` package.
2. A frecency scorer — hand-rolled/adapted, comparable in scope to granted's ~100-line `pkg/frecency`.
3. A git-status source — [`go-git/go-git`](https://github.com/go-git/go-git) (real, very active: 7,695
   stars, last push 2026-09-01, "a highly extensible Git implementation in pure Go") or simply shelling
   to `git status --porcelain`.
4. Content grep — the stdlib `regexp`/`WalkDir` liam already has, or a subprocess call to an external
   grep binary (ripgrep, `sift`, `ag`) for speed.

That's three to four separate dependencies (plus glue and tuning) replicating what fff ships as one
resident-process library with a single warm index.

## 5. A hand-written cgo binding against `fff-c`, scoped to darwin/linux amd64/arm64

Follow-up investigation, prompted by a direct question about what building the §1 `cgo` path would
actually look like, scoped to liam's real target platforms (darwin/linux, amd64/arm64 — no Windows).
Sources: `crates/fff-c/Cargo.toml`, `crates/fff-c/cbindgen.toml`, and the generated header
[`crates/fff-c/include/fff.h`](https://github.com/dmtrKovalenko/fff/blob/main/crates/fff-c/include/fff.h)
(all fetched directly via `gh api`, 2026-09-01), plus `dmtrKovalenko/fff`'s GitHub Releases assets.

**The C ABI is well-shaped for FFI, including Go.** `fff-c` (`crate-type = ["cdylib"]`) exposes an opaque
`void *` instance handle (`fff_create_instance_with(&FffCreateOptions)`, destroyed via `fff_destroy`),
with every call returning a heap-allocated `FffResult` envelope (`success`/`error`/`handle`/`int_value`).
The functions liam would call: `fff_search` (fuzzy find, frecency-ranked), `fff_live_grep`/`fff_multi_grep`
(content search, `mode` 0=plain/1=regex/2=fuzzy), `fff_refresh_git_status`. Rather than requiring callers
to replicate exact C struct layouts, it ships ~40 `fff_*_get_*` accessor functions
(`fff_file_item_get_relative_path`, `fff_grep_match_get_line_content`, `fff_grep_match_get_git_status`,
etc.) — clearly designed with non-Rust FFI consumers in mind, which would carry over cleanly to a Go
wrapper. Memory management is manual throughout (`fff_free_result`, `fff_free_search_result`,
`fff_free_grep_result`, `fff_free_string`, `fff_destroy`, each freed separately) — a Go wrapper would need
a `Close()`/`defer` pattern or `runtime.SetFinalizer` at each layer.

**Prebuilt shared libraries already exist for exactly liam's four targets.** Checked
`dmtrKovalenko/fff`'s latest GitHub Release assets directly (`gh api repos/dmtrKovalenko/fff/releases`):
it publishes `c-lib-aarch64-apple-darwin.dylib`, `c-lib-x86_64-apple-darwin.dylib`,
`c-lib-aarch64-unknown-linux-gnu.so`, and `c-lib-x86_64-unknown-linux-gnu.so` (each with a `.sha256`
checksum; musl variants also exist for a fully static/Alpine-friendly link, if that ever mattered). This
means liam would **not** need a Rust/cargo toolchain in its own build pipeline — just download and verify
the matching prebuilt artifact per target at release time, the same shape as however `fff-mcp` itself is
already fetched.

**Remaining real costs, scoped to darwin/linux only (materially smaller than a Windows-inclusive
estimate):**

1. `CGO_ENABLED=1` plus a working C compiler at Go build time — trivial on both OSes (Xcode CLT on
   macOS, gcc/clang preinstalled on virtually any Linux CI image). The genuinely painful part of cgo
   distribution is Windows/MSVC, which is out of scope here entirely.
2. Native-per-target builds instead of single-host cross-compilation — cgo generally can't cross-compile
   as cleanly as pure Go's `GOOS`/`GOARCH` matrix, so this wants 4 native CI runners (macOS arm64, macOS
   x86_64, Linux amd64, Linux arm64). GitHub Actions has native runners for all four today — a CI-matrix
   change, not a blocker.
3. The `.dylib`/`.so` must be locatable at runtime — bundle it alongside the liam binary and set an rpath
   (`-Wl,-rpath,@executable_path/...` on macOS, `-Wl,-rpath,$ORIGIN/...` on Linux). Well-trodden pattern
   for any Go tool linking a native library via cgo (e.g. `libgit2`, DuckDB bindings), not exotic.
4. Version pinning to a specific fff release tag's artifacts, re-vendored on updates — a cost liam
   already partially carries today regardless of integration path (`internal/mcp/fff.go`'s own comment
   notes fff-mcp's tool names/schemas were "verified live... not from any doc," i.e. liam is already
   exposed to fff version drift).

**Revised read vs. §1's blanket "real work" framing**: for liam's actual four targets, this is bounded,
concrete engineering work — not open-ended. It trades away Go's simplest "one static binary, trivial
cross-compile" story for "bundle a native shared library per platform + a native-per-arch CI matrix," but
that trade is now a known, scoped cost rather than a hypothetical one.

## Takeaways

- This confirms and sharpens `fff-integration.md`'s finding: not only is there no published Go binding
  for fff, there is no single Go-native library — nor an obvious pair — that covers fff's actual feature
  combination (fuzzy find + frecency + git-awareness + grep, resident-process fast). The `cgo`-via-`fff-c`
  path the fff README itself points to remains unbuilt (nobody has published one) — but per §5, building
  it for liam's own darwin/linux amd64/arm64 targets is bounded, scoped work, not open-ended, since fff's
  own release pipeline already publishes the prebuilt shared libraries needed.
- For each *individual* capability, real and actively maintained Go libraries do exist (`sahilm/fuzzy`,
  fzf's `src/algo`, `go-git/go-git`, `charlievieth/fastwalk`) — so "Go-native" is achievable, just not as
  a single drop-in.
- This turns liam's `find`/`grep` architecture question (raised in #97) into four concrete options, not
  two: (a) keep the MCP/subprocess connection to `fff-mcp`, (b) keep/harden the existing stdlib
  `WalkDir`+`regexp` fallback as the sole implementation, (c) hand-assemble a composite Go implementation
  from the narrower libraries cited in §2–4 — real integration and tuning work (frecency weighting,
  git-dirty boosting, index warm-up), not a one-line dependency swap — or (d) a purpose-built `cgo`
  binding against `fff-c` (§5), trading Go's single-static-binary/trivial-cross-compile story for a
  native-per-arch CI matrix and bundling a shared library per platform, in exchange for fff's actual
  feature set with no MCP/subprocess hop. No option found here is a costless like-for-like replacement
  for fff itself.
