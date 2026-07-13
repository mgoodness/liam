# Ctrl+C interrupts the current turn; Ctrl+D quits only on an empty buffer

Raw mode (issue #19, ADRs 0002/0005) disables both `ISIG` and `ICANON`, silently changing what Ctrl+C and Ctrl+D mean without either being scoped in that ticket's own acceptance criteria. This records the behavior settled on afterward.

**Ctrl+D**: matches canonical mode's actual convention (the same one bash and Python's REPL follow) — it only ends the session on an empty input buffer. With partial typed input, it submits what's been typed so far, the same as pressing Enter, rather than discarding it and ending the session. The prior unconditional-quit behavior was a real regression: a habitual Ctrl+D on partial input would silently end the whole session instead of submitting.

**Ctrl+C**: v0.1 inherited `SIGINT`'s default disposition — Ctrl+C could kill the whole process at any time, including mid-turn, with no graceful handling at all. Raw mode removes that entirely; before this decision, a hung `bash` call had no recourse short of killing the terminal externally. Ctrl+C now interrupts the current turn via a concurrent input reader canceling a `context.Context` threaded through both `Provider.Complete` and every tool `Handler` (each already takes a `ctx.Context` parameter, so the cancellation point already existed structurally) — abandoning whatever's in flight and returning directly to the prompt, the same way Ctrl+C already works during active generation elsewhere. Tool calls that completed earlier in the same turn stay in history; the interrupted call and anything not yet started are dropped without feeding a synthetic result back to the model — there's no value in a round-trip reaction to a cancellation the user is actively waiting out.

## Considered Options

- **Ctrl+C cancels just the current tool call**, feeding a "cancelled" result back to the model and letting it react before returning to the prompt. Rejected: fights the actual urgency of pressing Ctrl+C by requiring another full model round-trip first.
- **Ctrl+C abandons the whole turn** (chosen) — matches an already-familiar convention, returns control immediately.
- **Leave mid-flight interruption unsupported**, Ctrl+C only taking effect between turns as before. Rejected: a CLI agent with no way to stop a hung `bash` call is a worse regression than the cost of building the fix.
