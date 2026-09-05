# Loop-level Continuation guard forces retries on premature stops

`agentDone` (#102) shipped as a pure observer; a self-hosted trace later showed liam stopping mid-investigation with no `write`/`edit` ever run, and nothing could reject that stop. #210 adds `ShouldContinue` (type `agent.ContinuationGuard`), a plain Go func field on `agent.Loop` — deliberately **not** a Hook — consulted in `Run`'s no-tool-calls branch: on rejection it injects a synthetic `user`-role message and loops again, bounded by a `MaxContinuations` cap (default 3, `<=0` uses the default) enforced independently of the guard's own verdict. Tracked separately from `Run`'s own (compaction-rewritable) message history so the guard's view of "what happened this invocation" survives mid-invocation compaction. `cmd/liam/main.go` wires `agent.DefaultShouldContinue(registry)` as the concrete default: reject the stop as long as no `SideEffectWrite`-classified tool call has run yet this invocation, up to the same `MaxContinuations` cap. No config toggle exists to disable it.

This is deliberately *not* built as a Hook: ADR-0004 already shipped and pulled a Hook-shaped gating mechanism (`beforeTool`) after zero real configured usage, so this is pure in-process control flow instead, with none of a Hook's subprocess/JSON-stdin/fail-open surface. Shipping only a bare extension point risked repeating that exact "unused plumbing" fate, so a real default heuristic is wired up immediately rather than left for a hypothetical future caller.

## Considered Options

- Extend `agentDone`'s Hook contract with a `Decision`-style return (matching `beforeTool`'s shape) — rejected: `agentDone` was just shipped and reviewed specifically as a pure observer; reusing it blurs that already-settled contract.
- Ship `ShouldContinue` as a bare nil-by-default extension point with no wired default — rejected: repeats the "zero real configured usage" failure that got `beforeTool`/`userPromptSubmit` removed.
