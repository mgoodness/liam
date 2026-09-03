# `web_fetch` internal distillation: dedicated required model, additive parameter, no domain-allowlist

liam's `web_fetch` gains an optional parameter that, when present, distills the fetched content down to an answer for a specific question via a **dedicated small/fast model**, not the session's main model — confirmed as Claude Code's own deliberate design for its `WebFetch` tool (a two-tier cost/latency mitigation, not an afterthought), and the only precedent found: all five standing peers (pi.dev, pi-go, jcode, kit, crush) return raw capped content with no internal model call at all.

liam departs from Claude Code's actual shape in two ways. First, the parameter is **additive, not a replacement** — when omitted, today's raw-content-capped-at-8000-chars behavior is unchanged, so the calling model chooses per-call whether it wants raw or distilled. Claude Code's `WebFetch` has no such choice; it always distills, which is *why* it needs a hardcoded allowlist of trusted documentation domains to skip the model call for known-safe sources. liam doesn't adopt that allowlist — it would be solving a problem liam's own additive-parameter design doesn't have, at the cost of maintaining a curated domain list.

Second, the dedicated model is configured via a single, required, purpose-specific field (`provider.webFetchModel`) rather than a general named-model-roles system. If unset, the distillation parameter simply isn't offered — matching `web_search`'s existing `EXA_API_KEY`-gated graceful degradation. pi-go's `config.Roles` (already wired to a `CompressionRole` for its own compaction) is real precedent for a fuller roles system, and compaction wanting the same cheap-model treatment someday is a plausible second consumer — but that's not built yet, so a general system isn't justified now. This follows the same "generic but thin, only when a second consumer exists" reasoning as ADR-0008's Credential store.

## Considered Options

Require a query on every call, replacing raw mode entirely (Claude Code's actual shape). Rejected: forces every fetch through the model-gate, adding cost/latency even when the calling model genuinely wants raw content (e.g. a raw JSON API response, or a page to search line-by-line itself) — a regression for that case, not an improvement.

A hardcoded fallback model if `provider.webFetchModel` is unset. Rejected: bakes in an opinion about which model is "cheap" that goes stale and doesn't obviously generalize across future providers (#100's Copilot, ADR-0008).

A full named-model-roles system now. Rejected: no second consumer exists yet to justify the general mechanism over a purpose-specific field.

## Consequences

liam's tool-call cost accounting now includes an internal model call triggered by a tool the user didn't directly ask for a second model invocation from — rolled into the session's existing cost tracker transparently, same as compaction's summarization call, not a separate category. If compaction (or anything else) later wants its own dedicated cheap model, `provider.webFetchModel`'s shape is the template to generalize from, not a one-off to work around.
