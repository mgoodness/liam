# liam ships no built-in permission system (supersedes ADR-0003)

liam adopts pi.dev's "no built-in sandbox, no built-in permission prompts" architecture: built-in tools run with the harness process's own OS permissions, with no `manual`/`auto` mode distinction, no `--yolo` flag, no `permissions.commands` matcher, and no interactive prompt flow of any kind ([issue #41](https://github.com/mgoodness/liam/issues/41), Permissions section). This reverses [ADR-0003](0003-permission-prompts-over-no-sandbox.md), which recorded liam's original permission-prompt system as a deliberate *divergence* from pi.dev — that system is now removed entirely, adopting pi.dev's own precedent rather than diverging from it.

Users who want tool-call gating (confirmation-before-shell, deny-by-default policy, etc.) build it themselves as a `beforeTool` Hook — liam's existing DIY escape hatch, mirroring pi.dev's own answer via its extension system, just as a shell script instead of embedded TypeScript. `liam.jsonc`'s prior self-protection convention (writes to it always prompt, even in `auto` mode) is dropped too: it only made sense as a guard against silent permission-escalation, and there's no longer a permission system to escalate within.

The `Safety.Permission` field on the `Tool` interface (`allow`/`prompt`/`deny`), shipped as scaffolding in #44/PR-75 specifically for the system this ADR removes, is dropped along with it — `Safety` now carries only `SideEffect`, which stays useful for Trace's audit categorization independent of any gating decision.

## Considered Options

Keep a reduced permission system — e.g. only the `liam.jsonc` self-protection, or only shell-command gating — rather than full removal. Rejected for consistency: a partial system carries the same "easy to mistake for a real security boundary" risk pi.dev's own docs warn against (ADR-0003), while still requiring liam to build, document, and test a subsystem nobody asked to keep half of.
