# liam's Subagent tool: in-process delegation, no isolation, no recursion

liam adds a **Subagent** tool (#101) — a nested, in-process instance of the agent loop the model can spawn mid-session to delegate a bounded sub-task, following `charmbracelet/crush`'s design (liam's closest peer) rather than pi-go's. pi-go runs each subagent as a genuinely separate OS process inside a real `git worktree add`-isolated checkout, with its own nesting-depth-aware concurrency budget born from a real rate-limit incident — a "batteries-included agent runtime" shape. crush instead runs a subagent in-process, sharing the parent's toolset and working directory directly, with no file-conflict or locking mechanism at all. liam follows crush: in-process, matching liam's own single-binary, no-client/server-split identity (ADR-0007 already rejected a heavier third-party framework for the same reason), and with **no built-in concurrency-safety mechanism** — consistent with, not an exception to, ADR-0004's existing "no built-in gating" position.

A Subagent gets a fresh, independent context budget (not shared with the parent's remaining window) so delegating an expensive exploration doesn't consume the very budget it's meant to protect, and inherits the parent's configured model/provider rather than selecting its own (a fast-follow, #178, tracks letting it pick a different model later). It cannot itself spawn another Subagent — a flat rule, not crush's config-gated recursion, since liam has no per-agent-type config system to gate it with and unbounded nesting is exactly what caused pi-go's own rate-limit incident. Trace's (#63) audit schema is the intended place subagent tool-call attribution gets recorded; this is why #101 is blocked by #63 rather than inventing separate, temporary bookkeeping now.

## Considered Options

Out-of-process, git-worktree-isolated subagents (pi-go's shape). Rejected: categorically heavier than liam's architecture — a second OS process, worktree lifecycle management, and its own concurrency-budget subsystem, none of which liam's single-binary identity calls for.

A built-in file-conflict/locking mechanism between a Subagent and its parent. Rejected: would reopen ADR-0004's "no built-in gating" position for one feature specifically, for a risk liam's closest peer (crush) doesn't mitigate either.

Config-gated recursive subagent spawning (crush's approach). Rejected: real complexity (pi-go's nesting-depth concurrency-halving exists because of it) that isn't justified without the per-agent-type config infrastructure crush has and liam doesn't.

## Consequences

A Subagent and its parent (or two Subagents) can race on the same file with nothing stopping them — an accepted risk, same spirit as ADR-0004, not a gap to close later by default. Whoever builds #101 should design its attribution scheme to slot into Trace's (#63) schema once that lands, rather than the reverse.
