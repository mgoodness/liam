# Claude Code's Auto Mode

Research triggered mid-map by the user asking for something like Claude Code's Auto Mode in liam, a child topic of the [liam wayfinder map](https://github.com/mgoodness/liam/issues/1). Feeds a new ticket, "Decide: liam's Auto Mode scope and design."

Source: [code.claude.com/docs/en/permission-modes](https://code.claude.com/docs/en/permission-modes), fetched directly (official docs, not a secondary write-up).

## What Auto Mode actually is

Auto mode is one of six Claude Code permission modes (`default`/Manual, `acceptEdits`, `plan`, `auto`, `dontAsk`, `bypassPermissions`). It is **not** just "ask fewer questions" — it's a real-time safety architecture:

- A **second model** (the classifier — Sonnet 5 by default, independent of the session's own model) reviews each action that isn't a plain read or an in-working-directory file edit, **before** it runs.
- The classifier evaluates against a large, versioned, default rule taxonomy of **blocked** categories (curl-pipe-to-bash, secrets leaving the repo, production deploys, force pushes, `git reset --hard`, IAM/infra changes, disabling CI checks, printing live credentials, etc. — dozens of specific patterns) and **allowed** categories (local file ops, reading `.env` and using its credentials against the matching API, read-only HTTP, installing declared dependencies, pushing to any branch of the working repo, etc.).
- User-stated boundaries in conversation ("don't push", "wait for my review") are treated as block signals the classifier re-checks against the live transcript.
- **Fallback**: 3 consecutive blocks or 20 total blocks in a session pauses auto mode and falls back to prompting. A non-interactive (`-p`) run with no prompt fallback just doesn't run the blocked action and keeps going.
- **Subagents** get classifier review at spawn (task description), during (each action), and at completion (a full-history review that can prepend a warning).
- Auto mode is also what "nudges [the model] to keep working without stopping for clarifying questions" — but this nudge is a small, separate piece of the whole system, not the system itself.
- Cost/latency: each non-trivial action costs a classifier round-trip against a second model.

## What this implies for liam

Liam's own permission model (ticket #22, already resolved) is a much simpler, static, per-`SideEffect`/per-tool `allow`/`prompt`/`deny` scheme with a `--yolo` full-bypass flag — no classifier, no dynamic rule taxonomy, no per-action model call. Replicating Claude Code's Auto Mode *as specified* would mean:

- A second Provider call (via the same `Provider` interface, ticket #13) per non-trivial tool call — real cost/latency liam doesn't currently pay anywhere else.
- A rule taxonomy to define and maintain (liam has no equivalent "trusted infrastructure" or organization-policy layer to hang this off of — liam is a single-user local harness, not a fleet-managed enterprise tool).
- Fallback-threshold and boundary-tracking state machinery.

None of that is decided by anything already on the map. The one piece that's cheap and consistent with what's already locked: the "don't stop for clarifying questions" behavioral nudge, which is just a system-prompt-level instruction, unrelated to the classifier machinery.

## Open question this doesn't answer

Whether liam v1 wants: (a) just the clarifying-questions nudge, (b) a full classifier-review system matching Claude Code's actual architecture, or (c) something in between (e.g. reusing #22's static rules more aggressively rather than adding a live classifier) is a real design decision for the user, not something this research resolves.
