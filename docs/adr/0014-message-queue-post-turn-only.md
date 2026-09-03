# Message queue: post-turn injection only, Escape drops the queue

liam lets a user type and submit a follow-up message while the agent is still working (#108), queued and injected once the current turn naturally concludes (no more tool calls) — not woven into the in-progress turn mid-stream. This is deliberately narrower than pi.dev's cited prior art, which has two distinct queue types: "steering" (injected at each tool-call boundary, mid-turn) and "follow-up" (injected only once the turn would otherwise end). liam ships only the follow-up shape.

The reason is a real, well-evidenced risk, not caution for its own sake: every peer surveyed that implements mid-turn steering — including Claude Code's own CLI — has hit the same defect class at the tool-call-boundary seam (queued messages derailing ongoing work, getting misread as replies to output the user hadn't seen yet, idle-flag races). Claude Code's own desktop app doesn't attempt mid-turn steering at all, waiting for full turn completion instead — a parity gap Anthropic's own users are tracking as unresolved. liam's actual want ("don't force a cancel just to add input") is fully satisfied by post-turn injection alone; steering's marginal benefit isn't worth inheriting a known, still-unsolved architectural problem for a v1.

Multiple messages can be queued, injected in submission order as separate consecutive turns once the current one ends — no drain-mode configuration needed (pi.dev's `"all"`/`"one-at-a-time"` choice only existed to arbitrate *when* mid-turn injection happens, which this design avoids entirely). The input stays active while the agent works; a submitted-but-not-yet-injected message renders in the transcript with a distinct "queued" marker.

Escape — liam's only cancel action today, "one unified mechanism, no separate interrupt logic" — drops queued messages along with the in-flight turn. This was not the first answer considered: the initial recommendation was that queued messages should survive a cancel, on the reasoning that they're separate, deliberate input unrelated to what got cancelled. Checking actual behavior across peers reversed that: where confirmed, hard-cancel actions (Codex's and Copilot CLI's `Ctrl+C`) clear the queue too — the real pattern is two *separate* actions (a light clear-queue-only, a hard cancel-everything), not one action that discriminates. liam doesn't have that lighter action yet (tracked separately, #190); until it does, Escape must clear both, matching the confirmed hard-cancel behavior rather than inventing an unproven third option.

## Considered Options

pi.dev's full two-queue (steering + follow-up) design. Rejected for v1: the steering half is the exact shape every peer implementing it has had architectural trouble with; liam's core want doesn't require it.

Configurable drain mode (`"all"` vs `"one-at-a-time"`). Rejected: only pi.dev's steering/follow-up split created the need to arbitrate this; without steering, draining the whole queue in order is unambiguous.

Queued messages surviving an Escape cancel. Rejected on reflection: no peer confirmed to work this way — where confirmed, cancelling drops the queue too, via a separate hard-cancel action distinct from a lighter queue-only clear liam doesn't have (yet).

## Consequences

liam's message queue can't weave input into an in-progress turn the way steering-based tools attempt to — a deliberate, evidence-based scope limit, not an oversight. A lighter clear-queue-without-cancelling action (#190) is explicitly open for later, once there's a concrete keybinding to hang it on.
