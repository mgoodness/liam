# Built-in Tool-Call Gating

liam's built-in tools never refuse, block, or condition a call on anything the harness decides for itself. If a tool call is going to be blocked, that's a decision a user made — via a `beforeTool` Hook they wrote — not a decision baked into `write`, `edit`, `bash`, or any other built-in tool's own logic.

## Why this is out of scope

[ADR-0004](../docs/adr/0004-no-built-in-permission-system.md) commits liam to shipping no built-in permission or gating system of any kind: no `manual`/`auto` modes, no sandboxing, no interactive prompts. Built-in tools run with the harness process's own permissions, full stop. The stated rationale (via pi.dev, cited in ADR-0004 and [ADR-0003](../docs/adr/0003-permission-prompts-over-no-sandbox.md)) is that a partial, harness-decided gate is easy to mistake for a real security boundary — the only mechanism liam offers for "should this call happen" is the one the user builds themselves as a `beforeTool` Hook.

A feature that has a *specific* built-in tool refuse a call under some condition it detects internally — a read-before-write check, a size threshold, a path blocklist, anything where the tool itself says no — is exactly the shape of decision ADR-0004 ruled out, just scoped to one tool instead of the whole toolset. Rejecting it isn't about the specific mechanism (a read-ledger, a size cap, whatever); it's that liam's tools don't get to say no on their own.

This does **not** rule out a tool *reporting* something informational in its result (e.g. "note: this file wasn't read first this session") that the model or a user's own Hook could act on — only an unconditional built-in refusal.

## Prior requests

- #76: "Consider: session-scoped read-before-write ledger for write/edit" — pi-go's `ReadLedger` refuses a blind write to a file the model hasn't `read` in the current session; rejected as a hard refusal for the reason above.
