# Research: agentskills.io Skill spec details

Ticket: [mgoodness/liam#6](https://github.com/mgoodness/liam/issues/6) (child of wayfinder map issue #1)

## Summary

Agent Skills is an open format — originally developed by Anthropic, now maintained
as a community standard at [agentskills.io](https://agentskills.io) with a
reference implementation at
[github.com/agentskills/agentskills](https://github.com/agentskills/agentskills).
A skill is just a directory containing a `SKILL.md` file (YAML frontmatter +
Markdown body), optionally bundled with `scripts/`, `references/`, and
`assets/`. Discovery/invocation is **not a fixed protocol** — the spec defines
the file format and leaves directory scanning and activation mechanics up to
each harness, but it publishes strong conventions (below) for interoperability.

## 1. File format: `SKILL.md`

Source: [agentskills.io/specification](https://agentskills.io/specification)

A skill is a directory; at minimum it must contain a `SKILL.md` file:

```
skill-name/
├── SKILL.md          # Required: metadata + instructions
├── scripts/           # Optional: executable code
├── references/        # Optional: documentation
├── assets/             # Optional: templates, resources
└── ...                 # Any additional files or directories
```

`SKILL.md` = YAML frontmatter (between `---` delimiters) + a Markdown body
(no format restrictions on the body).

### Frontmatter fields

| Field | Required | Constraints |
|---|---|---|
| `name` | **Yes** | 1–64 chars. Lowercase unicode alphanumeric + hyphens only. Cannot start/end with a hyphen or contain `--`. **Must match the parent directory name exactly.** |
| `description` | **Yes** | 1–1024 chars, non-empty. Must state both *what* the skill does and *when* to use it — this is the only signal the harness/model uses to decide activation, so keyword-rich descriptions matter. |
| `license` | No | License name or reference to a bundled license file. |
| `compatibility` | No | Max 500 chars. Environment requirements (target product, system packages, network access, e.g. "Requires git, docker, jq"). Most skills don't need it. |
| `metadata` | No | Arbitrary string→string map for client-specific extensions. Recommend namespacing keys to avoid collisions. |
| `allowed-tools` | No, **experimental** | Space-separated string of pre-approved tools the skill may use, e.g. `Bash(git:*) Bash(jq:*) Read`. Support varies by implementation. |

Unrecognized frontmatter keys are ignored by spec-compliant runtimes. Avoid
`<`/`>` characters in frontmatter values (injection risk into the system
prompt).

Minimal valid example:

```yaml
---
name: skill-name
description: A description of what this skill does and when to use it.
---
```

### Optional bundled directories

- `scripts/` — executable code (language depends on the host agent; Python/Bash/JS are common). Should be self-contained or document dependencies.
- `references/` — additional docs loaded on demand (e.g. `REFERENCE.md`); keep individual files focused/small since they're pulled into context only when needed.
- `assets/` — static resources: templates, images, data files/schemas.

Guidance: keep `SKILL.md` itself under ~500 lines / <5000 tokens; push
detail into `references/` files, referenced with relative paths one level
deep from `SKILL.md`.

A reference validator ships in the repo:
`skills-ref validate ./my-skill` — [skills-ref](https://github.com/agentskills/agentskills/tree/main/skills-ref).

## 2. Directory & discovery conventions

Source: [agentskills.io/client-implementation/adding-skills-support](https://agentskills.io/client-implementation/adding-skills-support), [agentskills.io/skill-creation/quickstart](https://agentskills.io/skill-creation/quickstart)

**The spec itself does not mandate where skill directories live** — it only
defines what goes *inside* a skill directory. However, the implementation
guide documents a widely-adopted convention for cross-client
interoperability, built around two scopes (project vs. user) and two path
styles (client-native vs. cross-client):

| Scope | Path | Purpose |
|---|---|---|
| Project | `<project>/.<your-client>/skills/` | Client's native/branded location |
| Project | `<project>/.agents/skills/` | Cross-client interoperability |
| User | `~/.<your-client>/skills/` | Client's native/branded location |
| User | `~/.agents/skills/` | Cross-client interoperability |

`.agents/skills/` has emerged as the de facto cross-client convention —
scanning it makes skills installed for *other* compliant tools automatically
visible to yours. Some implementations also scan `.claude/skills/`
(project- and user-level) for pragmatic backward compatibility, since many
existing skills were authored for Claude. Other seen locations: ancestor
directories up to the git root (monorepo support), XDG config dirs, and
user-configured paths. VS Code's default, per the quickstart, is
`.agents/skills/` relative to the project root.

**Discovery mechanics:**
- Scan each configured directory for *subdirectories containing a file
  literally named* `SKILL.md`.
- Skip non-skill dirs (`.git/`, `node_modules/`); optionally respect
  `.gitignore`; bound the scan (e.g. max depth 4–6, max ~2000 dirs).
- On `name` collisions: project-level skills override user-level skills
  (universal convention). Within the same scope, first-found vs. last-found
  is implementation-defined but should be consistent and logged.
- **Trust**: project-level skills come from the repo being worked in
  (potentially untrusted, e.g. a freshly cloned OSS repo). Recommendation:
  gate project-level skill loading on a trust check to prevent an untrusted
  repo from silently injecting instructions via a skill description.
- Cloud/sandboxed agents without local filesystem access need alternative
  provisioning: project-level skills travel with a cloned repo; user/org-level
  skills need external provisioning (config repo, uploaded packages, etc.);
  built-in skills can ship as static assets in the deployment artifact.
- Parsing should be **lenient**: warn-but-load on a name/directory mismatch
  or an over-length name; skip-and-log only when `description` is
  missing/empty or the YAML is entirely unparseable. A common
  cross-client compatibility issue is unquoted colons inside `description`
  breaking naive YAML parsers.

## 3. Invocation / activation mechanism

Source: [agentskills.io/home](https://agentskills.io) (overview), [agentskills.io/client-implementation/adding-skills-support](https://agentskills.io/client-implementation/adding-skills-support)

Skills load via **progressive disclosure**, three tiers:

| Tier | What loads | When | Cost |
|---|---|---|---|
| 1. Catalog | `name` + `description` only, for every discovered skill | Session/harness startup | ~50–100 tokens/skill |
| 2. Instructions | Full `SKILL.md` body | When the skill is "activated" | <5000 tokens (recommended) |
| 3. Resources | `scripts/`, `references/`, `assets/` files | Only if/when the loaded instructions reference them | Varies |

**This is model-driven, not harness-side keyword/regex matching.** The
harness's job is limited to steps 1 (build and expose the catalog) and
providing a mechanism for step 2 (deliver full content on request); the
*decision* of which skill is relevant to the current task is made by the
model itself, reading the catalog's descriptions and matching them against
the user's request/task — same general principle as a well-written tool
description.

Two concrete activation patterns exist across implementations, both valid:

1. **File-read activation** — the catalog includes each skill's `SKILL.md`
   absolute path; the model uses its ordinary file-read tool to open the
   file when it judges the skill relevant. No special infra required, but
   the model gets the raw file including frontmatter.
2. **Dedicated tool activation** (e.g. an `activate_skill(name)` tool,
   constrained to an enum of known skill names to prevent hallucinated
   names) — required when the model can't read files directly; also gives
   the harness control to strip frontmatter, wrap content in identifying
   tags (e.g. `<skill_content name="...">`), enumerate bundled resources
   without eagerly loading them, enforce permissions/consent, and track
   usage.

The catalog itself is placed either as a labeled system-prompt section (simplest,
works with any model with file access) or embedded in the dedicated
activation tool's description (keeps the system prompt cleaner, couples
discovery+activation). Skills can also be excluded from the catalog
entirely (disabled by user/policy, or opting out via a
`disable-model-invocation`-style flag) — filtered skills should be hidden,
not listed-then-blocked.

Separately, **users can force-activate** a skill explicitly (not model
decided) — commonly via a slash command or `$mention` syntax the harness
intercepts directly, bypassing model judgment.

Operational notes relevant to a coding-agent harness: exempt injected skill
content from context-window compaction/pruning (losing it silently degrades
behavior with no visible error); dedupe repeated activations of an
already-loaded skill within a session; and optionally allowlist a skill's
bundled-resource paths in the permission system so reading `scripts/*` or
`references/*` doesn't trigger a confirmation prompt per file.

## Sources

- [agentskills.io — Overview](https://agentskills.io) (progressive disclosure summary, client showcase, origin statement — "originally developed by Anthropic, released as an open standard")
- [agentskills.io/specification](https://agentskills.io/specification) (full frontmatter field table, directory layout, validation tooling)
- [agentskills.io/skill-creation/quickstart](https://agentskills.io/skill-creation/quickstart) (worked example, VS Code's `.agents/skills/` default, 3-stage discovery/activation/execution walkthrough)
- [agentskills.io/client-implementation/adding-skills-support](https://agentskills.io/client-implementation/adding-skills-support) (discovery directory conventions, collision/trust handling, activation mechanism details, context management guidance — this is the harness-implementer's guide)
- [github.com/agentskills/agentskills](https://github.com/agentskills/agentskills) (reference implementation / `skills-ref` validator; spec source of truth; open to community contribution)
