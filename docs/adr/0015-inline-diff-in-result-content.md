# Inline diff for `edit`/`write`: the diff *is* the tool's `Content`, not a separate metadata carrier

liam's `edit`/`write` tools produce a real, hunk-based diff (via `go-udiff`, matching `charmbracelet/crush`'s proven choice) instead of today's generic single-line summary. Unlike both cited precedents — crush carries structured `OldContent`/`NewContent` (or a pre-formatted diff string) in a separate result-metadata field; Claude Code carries `originalFile` plus a `structuredPatch` hunk array alongside its plain result — liam's diff *is* the tool's existing `Result.Content` field. No new metadata type.

This isn't a compromise; it follows from a real difference in shape. Both crush and Claude Code need a separate structured carrier because their plain-text tool output and their rich UI are different surfaces with different needs. liam's `Result.Content` is already a single plain string consumed identically by headless mode and the TUI — a unified diff is perfectly sensible plain-text output for headless mode, so there's nothing a second field would buy that the existing one doesn't already cover.

Format is always unified (line-prefixed `+`/`-`), never crush's width-based unified-vs-side-by-side switch — side-by-side needs the renderer to know render width, which headless mode doesn't have the way the TUI does, and `Content` is shared between both. Large diffs truncate at hunk boundaries with an "N more lines truncated" marker, not liam's existing raw ~8000-byte cap (#86) — a byte cutoff can slice a hunk in half, which reads as corrupted in a way truncating a wall of plain text doesn't.

## Considered Options

A separate `Result` metadata field carrying structured before/after content or hunks (crush's/Claude Code's actual shape). Rejected: solves a problem liam doesn't have — there's no second, richer UI surface `Content` isn't already serving.

Width-based unified-vs-side-by-side rendering (crush's `.Split()` mode). Rejected: no shared notion of "available width" between headless mode and the TUI, and `Content` is one string serving both.

liam's existing raw byte-count truncation (#86's ~8000-char cap), reused as-is for diffs. Rejected: cuts hunks mid-line, reading as corrupted rather than merely abbreviated.

## Consequences

Existing golden-file tests for `edit`/`write`'s current one-line summary output need updating to the new diff-shaped `Content`. `go-udiff` becomes liam's first diffing dependency — justified the same way `CODING_STANDARDS.md`'s ANSI-width-truncation example justifies a dependency (a non-trivial, already-solved problem), not a departure from the stdlib-first stance.
