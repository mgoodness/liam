# Config conventions of prior art (pi, kit, zero, OpenCode)

Research for [liam#8](https://github.com/mgoodness/liam/issues/8), a child ticket of the [wayfinder map](https://github.com/mgoodness/liam/issues/1).

## Pi (pi.dev)

- **Format:** JSON.
- **Locations:** global `~/.pi/agent/settings.json`; project-local `.pi/settings.json`.
- **Precedence:** "Project settings (`.pi/settings.json`) override global settings. Nested objects are merged" — a deep merge, not a full replace, so a project file only needs to set the keys it wants to change.
- Separately, Pi also loads `AGENTS.md` (project instructions) and `SYSTEM.md` (system prompt override) from `~/.pi/agent/`, parent directories, and the current directory, and a `models.json` for custom providers/models — these are adjacent to, not part of, the settings config.
- Source: [pi.dev/docs/latest/settings](https://pi.dev/docs/latest/settings), [pi.dev/docs/latest](https://pi.dev/docs/latest), [pi.dev](https://pi.dev/).

## kit (github.com/mark3labs/kit)

- **Format:** YAML or JSON (`.kit.yml` / `.kit.yaml` / `.kit.json`).
- **Locations, in priority order (highest first):**
  1. CLI flags
  2. Environment variables (`KIT_` prefix)
  3. `./.kit.yml` / `./.kit.yaml` / `./.kit.json` (project-local)
  4. `~/.kit.yml` / `~/.kit.yaml` / `~/.kit.json` (global)
- **Precedence:** README states it explicitly as `Options > KIT_* env vars > .kit.yml > per-model defaults (modelSettings/customModels) > provider-level defaults` — a flat override chain, CLI/env/file layered on top of built-in defaults, rather than a directory-walking merge.
- Source: [github.com/mark3labs/kit](https://github.com/mark3labs/kit) README.

## zero (github.com/Gitlawb/zero)

- Zero's config story is split into two mechanisms rather than one settings file:
  - **Project/personal instructions:** appended to the system prompt from the first `AGENTS.md`, `ZERO.md`, or `.zero/AGENTS.md` found walking from the git root down to the current directory (checked in that order per directory), general-to-specific. A personal `ZERO.md` at `config.UserConfigDir()/zero/ZERO.md` applies globally, ahead of any project guidance. Files are capped at 8 KiB each / 32 KiB total.
  - **Plugins:** discovered from `~/.config/zero/plugins/` (user scope) and `.zero/plugins/` (project scope), managed via `zero plugins`.
- No single structured (YAML/JSON) settings file is documented in the README beyond these instruction/plugin discovery paths.
- Source: [github.com/Gitlawb/zero](https://github.com/Gitlawb/zero) README.

## OpenCode (opencode.ai)

- **Format:** JSON or JSONC (`opencode.json` / `opencode.jsonc`), with an optional `$schema` field for editor validation/autocomplete against `opencode.ai/config.json`.
- **Locations, lowest to highest precedence:**
  1. Remote config (`.well-known/opencode`) — org-wide defaults
  2. Global config (`~/.config/opencode/opencode.json`)
  3. Custom config (path from `OPENCODE_CONFIG` env var)
  4. Project config — discovered by walking from the filesystem root down to the current directory, both direct `opencode.json(c)` files and files inside `.opencode/` directories
- **Precedence/merge:** deep-merged, not replaced — "later configs override earlier ones only for conflicting keys; non-conflicting settings from all configs are preserved." Direct project configs are merged root-to-current-directory, then `.opencode/`-nested configs are merged the same way on top — so a `.opencode/opencode.json` always wins over a same-level direct `opencode.json`, even if the direct file is closer to the working directory.
- Source: [opencode.ai/v2/docs/config](https://opencode.ai/v2/docs/config/).

## Pattern across all four

- **Format:** JSON/JSONC or YAML dominate; no TOML in this set.
- **Two-tier location (global + project)** is universal; kit and OpenCode also add env-var/CLI-flag and org-remote layers on top.
- **Merge, don't replace**, is the norm where documented (pi, OpenCode): nested/conflicting keys override, everything else from lower layers survives. kit's override chain reads flatter (whole-file precedence) rather than deep-merged.
- zero is the outlier: no single structured settings file, just directory-walked Markdown instructions plus a separate plugin-discovery convention.
