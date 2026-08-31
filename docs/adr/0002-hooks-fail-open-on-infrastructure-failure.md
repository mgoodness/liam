# Hooks fail open on infrastructure failure

A hook that times out, crashes before exiting, or whose command can't be found fails open — treated as an allow / no-op, with a logged warning — rather than blocking the tool call or lifecycle event it gates. Only an explicit non-zero exit from a hook that actually ran and returned counts as a deny; [issue #41](https://github.com/mgoodness/liam/issues/41)'s existing "non-zero on a blocking `beforeTool` hook = deny" rule only covers that case. This mirrors jcode's documented rationale ([docs/research/pi-go-jcode-prior-art.md](../research/pi-go-jcode-prior-art.md)): "a broken policy script should degrade to 'no policy' rather than brick every session... if you need fail-closed semantics, make the hook itself robust — it is your trust boundary, not [the harness]." Every hook process also receives `LIAM_HOOKS_DISABLED=1` in its environment, alongside the already-planned `LIAM_*` vars, so a hook that itself invokes `liam` headlessly doesn't recursively re-trigger hooks.

## Consequences

A misconfigured or crashing hook silently stops enforcing whatever policy it implements, rather than blocking work — hook authors are responsible for making their own script robust if they need fail-closed behavior for something security-sensitive.
