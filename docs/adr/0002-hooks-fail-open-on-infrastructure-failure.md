# Hooks fail open on infrastructure failure

A hook that times out, crashes before exiting, or whose command can't be found fails open — treated as a no-op, with a logged warning — rather than surfacing as a failure any differently than the hook simply not being configured. This mirrors jcode's documented rationale ([docs/research/pi-go-jcode-prior-art.md](../research/pi-go-jcode-prior-art.md)): "a broken policy script should degrade to 'no policy' rather than brick every session... if you need fail-closed semantics, make the hook itself robust — it is your trust boundary, not [the harness]." Every hook process also receives `LIAM_HOOKS_DISABLED=1` in its environment, alongside the already-planned `LIAM_*` vars, so a hook that itself invokes `liam` headlessly doesn't recursively re-trigger hooks.

**Update (2026-09):** this ADR originally distinguished fail-open from an explicit non-zero exit on a *blocking* `beforeTool` hook, which counted as a real deny. That blocking contract (issues #84/#102, later extended to a `userPromptSubmit` point too) was removed after shipping with zero real configured usage — see ADR-0004's note and `internal/hook`'s package doc. Every hook lifecycle point today (`sessionStart`/`sessionEnd`/`afterTool`/`agentDone`) is a pure observer, so this ADR's fail-open/non-zero-exit distinction now only affects what gets logged via `Warn`, not any tool call or lifecycle event's outcome.

## Consequences

A misconfigured or crashing hook silently stops observing, rather than blocking work or spamming errors — hook authors are responsible for making their own script robust if they need to rely on it actually running every time.
