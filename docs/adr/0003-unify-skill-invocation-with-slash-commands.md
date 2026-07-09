# Skills follow the agentskills.io format, with slash-command invocation as our own additive extension

For v0.2, liam's skills are directories conforming to the open [Agent Skills specification](https://agentskills.io/specification) (originally developed by Anthropic): `skill-name/SKILL.md` with required `name`/`description` frontmatter, optional `license`/`compatibility`/`metadata`/`allowed-tools` fields (parsed permissively — liam ignores any it doesn't act on, e.g. `allowed-tools` is moot under YOLO mode's no-confirmation stance), and optional bundled `scripts/`/`references/`/`assets/` directories the skill's body can point to via relative paths, needing no special handling since the model already reaches them via `read`/`bash` once it knows the path.

The open spec itself only defines *discovery* and *activation*: name+description loaded at startup, full body loaded when a task matches. It says nothing about explicit invocation — that's left to each client. So a discovered skill's `name` becoming its `/name` slash command (per ADR before this rewrite) is additive to the spec, not a deviation from it, the same way Codex and Claude Code both layer their own explicit-invocation UX on top of the same base format. Anything typed after the name is passed through as trailing argument text, not substituted into positional placeholders — still deliberately not pi's separate prompt-template mechanism with `$1`/`$2`/`$@` substitution.

`disable-model-invocation` isn't part of the open spec — it originates with Claude Code — but real-world skills already use it, so liam honors it anyway: a skill author's choice to make something explicit-invocation-only shouldn't get silently overridden just because the base spec doesn't mandate the field.

`name` validation follows the spec exactly, since it's also what determines the slash command: lowercase alphanumeric + hyphens only, ≤64 characters, no leading/trailing/consecutive hyphens, and must match the parent directory name. A skill failing validation is skipped with a warning, not a startup failure.

## Considered Options

- **Pi's separate systems** — Skills and prompt templates as two distinct discovery mechanisms and file conventions. Rejected: more moving parts (two formats, two discovery passes, a template-substitution engine), for a distinction liam's own users don't need.
- **Codex's unified model, layered on the open spec** (chosen) — matches what liam's own maintainer already uses daily in Claude Code (a skill's name serving both purposes, with an opt-out flag for model-invocation), stays interoperable with the broader Agent Skills ecosystem, and is simpler to implement than a second templating system.
