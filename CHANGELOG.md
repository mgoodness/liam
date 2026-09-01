# Changelog

## [1.0.0](https://github.com/mgoodness/liam/compare/v0.1.0...v1.0.0) (2026-09-01)


### Features

* add Provider interface, OpenRouter adapter, headless echo ([fe700ee](https://github.com/mgoodness/liam/commit/fe700ee74c53e1f3905d74f7655157e7ad6ae87b))
* **agent,session:** add compaction (/compact mechanism, sliding window + summarization) ([#92](https://github.com/mgoodness/liam/issues/92)) ([7223ba8](https://github.com/mgoodness/liam/commit/7223ba85efafd7d9f5cbc089c648b1c7fbc8193b))
* **agent:** add ProviderError.Kind-based retry policy ([#90](https://github.com/mgoodness/liam/issues/90)) ([83bb8b1](https://github.com/mgoodness/liam/commit/83bb8b150cea242a1d07e11e1e38e555ffcae72d))
* **cmd/liam:** add --version flag with ldflags/build-info fallback ([f7ed452](https://github.com/mgoodness/liam/commit/f7ed4525e3a35ea2e0ead92ea04eec93e5ff0dac))
* **config:** add JSONC config loading with precedence and schema container ([#74](https://github.com/mgoodness/liam/issues/74)) ([edcf312](https://github.com/mgoodness/liam/commit/edcf312ee8fa409688a28678210f358328f75cd0))
* **hook:** add Hooks system with 4 lifecycle points and beforeTool blocking ([#84](https://github.com/mgoodness/liam/issues/84)) ([13f6d9d](https://github.com/mgoodness/liam/commit/13f6d9d6ef52ca0006a2496b352be30795d8f469))
* **instructions:** add liam's fixed identity preamble ([#95](https://github.com/mgoodness/liam/issues/95)) ([#111](https://github.com/mgoodness/liam/issues/111)) ([d1d2899](https://github.com/mgoodness/liam/commit/d1d2899143fabcbdce21da002b04f20d0bd154cd))
* **instructions:** discover and assemble AGENTS.md/LIAM.md into SystemPrompt ([ccc1029](https://github.com/mgoodness/liam/commit/ccc10296ee61ff89181bd8ffba1f7d31bc91e60d)), closes [#56](https://github.com/mgoodness/liam/issues/56)
* **mcp:** add MCP client (stdio, tools capability) ([#87](https://github.com/mgoodness/liam/issues/87)) ([4351160](https://github.com/mgoodness/liam/commit/435116009009827e07764b8d0439a874cc3bed5b))
* **session:** add context-percentage and cost tracking ([#91](https://github.com/mgoodness/liam/issues/91)) ([ae95282](https://github.com/mgoodness/liam/commit/ae952827591d2b144d5a15726ae95aaa9d569aa5))
* **skill,tui:** Agent Skills discovery, activate_skill tool, trust-gating, and /skills ([#83](https://github.com/mgoodness/liam/issues/83)) ([d7f6b4b](https://github.com/mgoodness/liam/commit/d7f6b4beb9b2f863044d8b1ac2a7ac16b17c6369))
* **statusline:** add customizable statusLine status block ([#60](https://github.com/mgoodness/liam/issues/60)) ([#98](https://github.com/mgoodness/liam/issues/98)) ([1dd7c49](https://github.com/mgoodness/liam/commit/1dd7c49f345c1b5b7d473f23b160abe31594d024))
* **tool,agent:** Tool interface, agent loop dispatch, and core tools ([#75](https://github.com/mgoodness/liam/issues/75)) ([d18e4ee](https://github.com/mgoodness/liam/commit/d18e4ee9d9c8aef952e080852788ffdc1e7424c4))
* **tool:** add find/grep tools (fff-mcp + stdlib fallback) ([#88](https://github.com/mgoodness/liam/issues/88)) ([9cf4803](https://github.com/mgoodness/liam/commit/9cf4803fe06b9fead57f3e03cd91ce5e0c81bb75))
* **tool:** add web_search and web_fetch tools ([#89](https://github.com/mgoodness/liam/issues/89)) ([24a6775](https://github.com/mgoodness/liam/commit/24a677511fc2e20b6c18241e975c44544fba243d))
* **tool:** truncate bash/read output at ~8000 bytes, matching web_fetch ([#86](https://github.com/mgoodness/liam/issues/86)) ([58d10cd](https://github.com/mgoodness/liam/commit/58d10cdaeeb2fe6d7965203313c8608e6aeefc69))
* **tui:** add @-file-reference autocomplete and input history ([#94](https://github.com/mgoodness/liam/issues/94)) ([88e73ef](https://github.com/mgoodness/liam/commit/88e73ef5726995a4fd75895531cf2842b529557f))
* **tui:** add conversation-viewport scrolling ([#96](https://github.com/mgoodness/liam/issues/96)) ([2543fc0](https://github.com/mgoodness/liam/commit/2543fc0286366e3d27f42accc0ded1c5864fe329))
* **tui:** interactive TUI shell — layout, theming, textarea, /quit /clear Escape-cancel ([#81](https://github.com/mgoodness/liam/issues/81)) ([481d12e](https://github.com/mgoodness/liam/commit/481d12e0d7df42586c0abce2a1a9dbd3d6440e58))


### Miscellaneous Chores

* set next release to v1.0.0 ([fa732fe](https://github.com/mgoodness/liam/commit/fa732feb6eb03294a0727f1ac6e2c09727746e20))
