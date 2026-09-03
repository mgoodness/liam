# Session persistence: flat linear JSONL, not a tree structure or SQLite

liam persists Sessions to disk (#105) so quitting no longer discards a conversation — resumed via `-c`/`--continue` (the most recent Session for the current working directory). Prior art split three ways: pi.dev and pi-go (independently, different codebases) both use tree-structured JSONL with fork/branch/clone as first-class UX; crush uses SQLite instead, with migrations and a cgo-free driver. liam adopts neither's full shape: **flat, linear JSONL**, one file per Session, keyed by working directory under `$XDG_STATE_HOME/liam/sessions/<hash of abs repo path>/<session-id>.jsonl` — mirroring the per-repo-hash convention #99 already established for fff's frecency store.

This follows the same reasoning as ADR-0007 and ADR-0009: don't adopt a peer's heavier shape until liam's own use case earns it. Tree/branch support is a genuinely separate feature (liam's compaction model is explicitly linear today, with no slot for branching) — real scope beyond "don't lose my session on restart," left for a later ticket if ever wanted. SQLite's relational query benefits matter for crush's branching/todo-tracking needs; a flat append-log has no comparable need, so it doesn't justify a new dependency plus migration tooling.

A persisted Session stores its **post-compaction** state — whatever the live message history actually is at exit, not a separate archive of pre-compaction detail. Resuming puts the user back exactly where the model itself was, not a silently rehydrated version the model never had access to. Full historical audit is Trace's (#63) job, not this ticket's.

CLI surface for this ticket is `-c`/`--continue` only, matching liam's existing flat-flag surface (no new subcommand). An interactive picker (pi.dev's `-r`: search/sort/filter/rename/delete) is deferred to a fast-follow — real additional TUI work (a new dialog type) beyond the persistence mechanism itself.

## Considered Options

Tree-structured JSONL with fork/branch/clone (pi.dev, pi-go). Rejected for v1: requires liam's linear compaction model to grow a branching concept it doesn't have today — separable scope, not a prerequisite for basic persistence.

SQLite (crush). Rejected: no relational query need a flat append-log has, so no justification for the dependency + migration-tooling cost.

Persisting pre-compaction transcript alongside the live (post-compaction) one. Rejected: duplicates what Trace (#63) is for, and risks resuming into a state the model itself never actually had.

## Consequences

liam's session file format is now a real on-disk contract — changing it later means a migration story, not just a code change. The interactive session-picker UX (`-r`-equivalent) and tree/branch support are both explicitly open for later, separate tickets, not ruled out.
