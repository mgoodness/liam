# Coding standards

This document captures the conventions already implicit in the codebase, so
they don't have to be re-derived by reading ADRs and neighboring files on
every change or review. It documents what's here — it doesn't introduce new
rules. Where it and the code disagree, treat that as a bug in this document
and fix it.

`CONTEXT.md` is the canonical domain vocabulary; `docs/adr/` holds
architecture decisions with their full reasoning. This file is about *how
code is written*, not *what liam does*.

## Language and dependencies

Idiomatic Go, strongly prefer the standard library. Third-party
dependencies are exceptions taken deliberately and narrowly — the
Bubbletea/Lipgloss/Bubbles TUI stack, the official OpenRouter and MCP Go
SDKs, `golang.org/x/net/html` for Markdown conversion. A new dependency
should be justified the same way: no stdlib equivalent, or a stdlib
equivalent that would mean reimplementing a non-trivial, already-solved
problem (ANSI-aware string width/truncation, for instance).

No third-party assertion or mocking library, anywhere — tests use plain
`if got != want { t.Errorf(...) }` and hand-written fakes.

## Package and file organization

- One package per subsystem under `internal/`, named after what it *is*
  (`hook`, `session`, `statusline`), not what it does to something else.
- The package doc comment lives in the file most central to the package —
  usually the one sharing the package's name (`hook.go`, `session.go`) —
  and explains what the package is responsible for, not an index of its
  types.
- Within a package, a self-contained feature gets its own file plus a
  matching `_test.go` (e.g. `mention.go`/`mention_test.go`,
  `history.go`/`history_test.go`, `statusline.go`/`statusline_test.go`),
  even when most of its functions are methods on a type defined elsewhere
  in the package (see `internal/tui/mention.go`'s `*Model` methods). Don't
  let one file accumulate every feature just because they share a
  receiver type.

## Doc comments

Every exported identifier — and most unexported ones with any nuance —
gets a doc comment. The bar is higher than "what this does" (often
obvious from the name and signature); a good comment answers *why* it
exists, *why* it's shaped this way, or *what would break* if a caller got
it wrong:

- Cross-reference the originating issue/ticket/ADR when the shape of the
  code is a direct consequence of a decision made there (`// per ADR-0002`,
  `// (issue #58)`), so a reader can find the reasoning instead of just the
  rule.
- Call out non-obvious invariants and gotchas explicitly — e.g.
  `streaming *strings.Builder` in `internal/tui/tui.go` documents exactly
  why it must be a pointer (Bubbletea copies `Model` by value every
  `Update`/`View` call; a `strings.Builder` value panics if written to
  after being copied post-write).
- Prefer a comment that would let a reader make a good judgment call on an
  edge case the code doesn't explicitly handle, over one that just
  restates the signature.

Inline (non-doc) comments follow the repo-wide rule: explain the *why*
(a hidden constraint, a workaround, a subtle invariant), never the *what*.
Don't add a comment a well-named identifier already makes redundant.

## Errors

- Wrap with `fmt.Errorf("<package>: <action>: %w", err)`. The package-name
  prefix is a convention, not `errors.New`/`fmt.Errorf` enforcing it — see
  `internal/config`, `internal/skill` for the pattern in practice.
- A boundary that classifies failure into caller-actionable categories
  (rate-limited vs. invalid vs. unavailable, say) does it with a typed
  `Kind` on a custom error type (see `provider.ProviderError`), not
  sentinel errors or string matching.
- Fail open vs. fail closed is a deliberate, documented decision, not a
  default — see ADR-0002 for the precedent (hooks fail open on
  infrastructure failure, never on a real policy verdict) and match its
  reasoning when adding a new failure path with similar stakes.
- Don't add error handling for a scenario that can't happen given the
  caller's own guarantees. Validate at real boundaries (config parsing,
  external processes, HTTP responses) — not defensively between two
  functions in the same package that already agree on their contract.

## Testing

- Table-driven tests for anything with more than two or three cases worth
  covering; a `cases := []struct{...}{...}` with a `for _, tc := range
  cases { t.Run(tc.name, ...) }` loop, not a wall of near-duplicate test
  functions.
- Prefer a hand-written fake over a mock: a fake `Provider` that scripts a
  fixed `Event` sequence, a fake `Tool` that returns a canned `Result`, a
  fake `ContextLookup` with a canned map and a call counter. The fake
  should be exactly as complex as the interface it satisfies and no more.
- HTTP boundaries (OpenRouter, Exa, any future external API) are tested
  against `net/http/httptest` servers standing in for the real API — never
  live credentials in a test, and CI never needs API keys.
- Plain-text/deterministic-output tools use golden files:
  `testdata/*.golden`, read back via a small `readFile`/`readGolden`
  helper, with an `-update` flag convention for regenerating them
  (`internal/tool/websearch_test.go`, `internal/tool/search_stdlib_test.go`).
- Test *external, observable behavior* — what a function returns, what a
  fake-driven loop does with a given input, what a config string parses
  into — never internal implementation details of how the result was
  produced. If a test would break from a pure refactor that doesn't change
  behavior, it's testing the wrong thing.
- A test that needs to avoid a real sleep or timeout gets an injectable
  seam for it (`agent.Loop.Backoff`, `statusline.commandTimeout`,
  `tui.Model.statusDebounce`) rather than actually waiting — but only where
  a test needs it; don't add the seam speculatively.
- Every new exported behavior gets both a happy-path test and its
  documented edge cases (empty/zero input, the specific error paths its
  own doc comment calls out) — not just enough to hit the acceptance
  criteria that prompted it.

## Responding to code review

A `/code-review`-style pass separates two axes — Standards (repo
conventions) and Spec (does it match the issue) — and reports findings; it
does not fix them itself. The fixing is a separate, deliberate step, and
the bar for that step is:

- A **hard violation** (breaks a documented standard in this file or in an
  ADR) always gets fixed before the change lands.
- A **small, local Standards-axis smell** — a long parameter list, a data
  clump, duplicated logic shape across two files, a misleading comment —
  gets fixed in the same change too, not left as a noted-but-unaddressed
  judgment call. "Judgment call" describes the *severity label*, not
  permission to defer the fix: these are cheap to fix precisely because
  they're small and local, and leaving them for "later" is how they
  accumulate. Extract the shared shape, rename the thing, bundle the
  clump, correct the comment — then move on.
- A smell that's genuinely **not small or local** (it would require
  restructuring a stable, unrelated, already-tested module; or fixing it
  would itself be a second unrelated intent worth its own commit) is the
  one legitimate case for noting it and deferring — and it should be
  turned into a tracked follow-up (an issue, not just a comment in the
  review output), not silently dropped.
- Speculative-generality suggestions (an abstraction the review imagines a
  future need for, not one the current diff demonstrates) are the one
  category that's fine to decline outright.

This applies regardless of who or what ran the review — the review output
itself carries no memory of this policy, so anyone acting on review
findings in this repo should apply it explicitly.

## Construction patterns

- A type with required dependencies and optional ones takes the required
  ones as `New(...)` constructor arguments and the optional ones as
  fluent `With*` builder methods returning a modified copy (see
  `tui.Model`'s `WithMCPLoader`/`WithSystemPrompt`/`WithFindSearcher`/
  `WithCwd`). A zero-value receiver (no `With*` call) must leave the
  feature cleanly disabled, never panic or behave inconsistently.
- Config types are plain structs with `json` tags, deep-merged generically
  (`internal/config/merge.go`'s `mergeMaps`) rather than each field having
  its own merge logic — a new config field needs no merge-side code at
  all, just the struct field.
- An unimplemented/future config section is a stub struct (`PluginsConfig
  struct{}`) with a doc comment noting what will populate it, not a
  `TODO` comment or an omitted field.

## Naming and vocabulary

Use `CONTEXT.md`'s terms exactly, including its "avoid" lists — `Hook` not
"plugin/callback/trigger", `Session` not "conversation/chat", `Tool` not
"capability/action/function". A PR that introduces new domain vocabulary
should extend `CONTEXT.md`, not just use a new term inline.

## Commits

Conventional Commits (`feat(scope): ...`, `fix(scope): ...`,
`refactor(scope): ...`, `chore(scope): ...`, `docs(scope): ...`), scoped
to the package or subsystem touched. A change with more than one real
intent (a feature plus an unrelated refactor it surfaced, say) is split
into separate commits rather than bundled into one — the refactor should
stand on its own and pass tests independently of the feature that
motivated it.

## When to write an ADR

A new one goes in `docs/adr/` when a decision trades off real
alternatives and the reasoning would otherwise only live in a commit
message or a closed issue — see the existing five for the bar (fail-open
hooks, no built-in permission system, output truncation). A decision
that's simply "the obvious way to implement what the spec already says"
doesn't need one.
