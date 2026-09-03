# Copilot as liam's first OAuth-backed Provider: case-by-case reverse-engineered-auth policy, and a generic Credential store

liam adds GitHub Copilot as a second `Provider` (#100), authenticating via the same undocumented mechanism ~30 other active open-source coding-agent projects already use — a GitHub OAuth device-code flow under VS Code's own reverse-engineered client ID (`Iv1.b507a08c87ecfe98`), exchanged for a short-lived session token via `api.github.com/copilot_internal/v2/token` (also undocumented), then completions against `api.githubcopilot.com/chat/completions`. GitHub has never sanctioned third-party access this way, but has also taken no visible enforcement action against this ecosystem (which includes `charmbracelet/crush`, liam's own closest peer project per ADR-0007) despite years of scale and visibility — a "could change without notice" risk, not a "will get you banned" risk. liam accepts this **case-by-case**, not as a blanket policy: Copilot clears the bar on today's evidence, but each future subscription-backed provider (Claude, ChatGPT/Codex, Gemini — all real prior art per `docs/research/pi-go-jcode-prior-art.md`) gets its own fresh evaluation rather than inheriting Copilot's precedent automatically.

This breaks [issue #19](https://github.com/mgoodness/liam/issues/19)'s standing v1 convention that every external-service credential is read from its own environment variable, never persisted by liam itself — that assumed a static secret, which an OAuth refresh token isn't. liam gains a generic (not Copilot-specific) **Credential** store: a `Provider`-keyed refresh token, persisted as a plaintext file with `0600` permissions under `$XDG_STATE_HOME/liam/credentials/<provider>.json`, acquired via a minimal, hand-rolled `liam auth login <provider>` subcommand (liam's first subcommand — full Cobra migration is tracked separately, [#176](https://github.com/mgoodness/liam/issues/176), not bundled into this). Provider selection is explicit only: a new `provider.name` config field, defaulting to `"openrouter"`, no auto-detection between whichever credential happens to be present.

## Considered Options

Blanket-accept reverse-engineered/client-mimicking auth as a standing policy for any future subscription-backed provider. Rejected: today's precedent for Copilot specifically doesn't transfer to a different vendor's endpoint stability or tolerance — each one needs its own evaluation.

OS keychain integration for Credential storage, instead of a plain permissioned file. Rejected: a real third-party dependency (`CODING_STANDARDS.md`'s narrow-exceptions bar) for marginal benefit over a `0600` file, absent a concrete threat model that demands it.

Auto-detecting the active provider from whichever credential is present. Rejected: creates an unresolvable ambiguity when both `OPENROUTER_API_KEY` and a stored Copilot credential exist simultaneously; an explicit `provider.name` field is deterministic.

Reusing an existing `gh auth login` session's token instead of running liam's own device-code flow. Not viable, not just undesirable — confirmed Copilot's token-exchange endpoint rejects tokens minted under `gh`'s own registered OAuth app; only the VS Code Copilot client ID's tokens are accepted.

## Consequences

liam now persists secret material on disk for the first time, outside its existing env-var-only convention — `$XDG_STATE_HOME/liam/credentials/` becomes something the harness should never log or include in any generic "show config" output. Whoever adds the next subscription-backed provider reuses the Credential store's acquisition/refresh/storage mechanics, but must re-run the risk-acceptance evaluation from scratch, not treat this ADR as blanket precedent.
