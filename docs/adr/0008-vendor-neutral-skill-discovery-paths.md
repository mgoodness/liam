# Skill discovery uses vendor-neutral paths, not liam-namespaced ones

liam's original Skill discovery paths — `$XDG_CONFIG_HOME/liam/skills/` globally, `.liam/skills/` per project — namespaced every Skill to liam itself. A Skill authored once for liam couldn't be picked up by any other Agent-Skills-compatible tool without copying or symlinking it per tool, even though the whole point of the open spec (ADR 0003) is that a Skill is portable across clients. The maintainer already maintains a personal, tool-agnostic Skill directory (`~/.agents/skills/`) for exactly this reason — populated with real Skills liam had no way to see.

The open Agent Skills spec deliberately says nothing about discovery paths at all — it's left entirely to each client — so there's no external standard being violated here or deferred to; this is purely liam's own convention to choose.

Skill discovery now uses three vendor-neutral locations instead of two liam-namespaced ones:

- **Global**, checked in this order (a later directory wins on a name collision — see `skill.Discover`): `$XDG_CONFIG_HOME/agents/skills/` (falling back to `~/.config/agents/skills/` when unset), then `~/.agents/skills/`.
- **Project**: `.agents/skills/` at the git repository root (or the given directory, if not inside a repository).

Two global locations are necessary, not redundant: `~/.agents/skills/` is the convention actually in active use across the broader agent-skills ecosystem today, and it doesn't respect `XDG_CONFIG_HOME` — so folding it into the XDG-compliant path would simply miss it for anyone (including the maintainer) already using it. Checking both, with `~/.agents/skills/` winning on a collision, means an XDG-respecting setup and the ecosystem's actual convention both work, and a deliberate override placed in the more actively-used location isn't silently shadowed.

The old liam-namespaced paths are dropped entirely — no fallback, no migration window. v0.2 had just shipped when this changed, so the realistic population of anyone depending on the old paths was effectively the maintainer alone; a third fallback location would have added real discovery complexity (three directories to reason about instead of two) to protect against a migration cost that, in practice, doesn't exist.

## Considered Options

- **Keep liam-namespaced paths, add vendor-neutral ones as extra fallbacks** — Rejected: three-plus locations to reason about per scope, for a migration window with no real users to protect.
- **Vendor-neutral paths only, XDG-compliant location alone (no `~/.agents/skills/`)** (considered) — simpler, one location per scope. Rejected: the ecosystem's actual convention doesn't respect `XDG_CONFIG_HOME`, so this would silently miss Skills already in active use, including the maintainer's own.
- **Two vendor-neutral global locations, `~/.agents/skills/` wins on collision, liam-namespaced paths dropped entirely** (chosen) — matches both an XDG-respecting setup and the ecosystem's real-world convention, with no migration complexity carried forward.
