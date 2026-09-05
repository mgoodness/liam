# Built-in Tool-Call Gating

liam's built-in tools never refuse, block, or condition a call on anything the harness decides for itself. There is no mechanism, built-in or Hook-based, for blocking a tool call at all (see ADR-0004's 2026-09 update: a `beforeTool` Hook shipped as the DIY escape hatch and was later removed with zero real configured usage) — a tool call either runs or it doesn't reach the tool at all (unknown name, malformed args).

## Why this is out of scope

[ADR-0004](../docs/adr/0004-no-built-in-permission-system.md) commits liam to shipping no built-in permission or gating system of any kind: no `manual`/`auto` modes, no sandboxing, no interactive prompts, and (as of its 2026-09 update) no Hook-based gating either. Built-in tools run with the harness process's own permissions, full stop. The stated rationale (via pi.dev, cited in ADR-0004 and [ADR-0003](../docs/adr/0003-permission-prompts-over-no-sandbox.md)) is that a partial, harness-decided gate is easy to mistake for a real security boundary.

A feature that has a *specific* built-in tool refuse a call under some condition it detects internally — a read-before-write check, a size threshold, a path blocklist, anything where the tool itself says no — is exactly the shape of decision ADR-0004 ruled out, just scoped to one tool instead of the whole toolset. Rejecting it isn't about the specific mechanism (a read-ledger, a size cap, whatever); it's that liam's tools don't get to say no on their own.

This does **not** rule out a tool *reporting* something informational in its result (e.g. "note: this file wasn't read first this session") that the model or a user's own Hook could act on — only an unconditional built-in refusal.

## Prior requests

- #76: "Consider: session-scoped read-before-write ledger for write/edit" — pi-go's `ReadLedger` refuses a blind write to a file the model hasn't `read` in the current session; rejected as a hard refusal for the reason above.
