# Changelog

## [1.1.0](https://github.com/mgoodness/liam/compare/v1.0.0...v1.1.0) (2026-09-03)


### Features

* **tui:** add a startup header banner, like Claude Code's ([#172](https://github.com/mgoodness/liam/issues/172)) ([1e02b8c](https://github.com/mgoodness/liam/commit/1e02b8c07a238643ff5ac375f2f52a9852a287fc))
* **tui:** add manual /compact slash command ([#127](https://github.com/mgoodness/liam/issues/127)) ([7d27d30](https://github.com/mgoodness/liam/commit/7d27d3065581eccb3acfcff287a92f5246264d06))
* **tui:** append a completion summary line after each turn ([#167](https://github.com/mgoodness/liam/issues/167)) ([a3c939a](https://github.com/mgoodness/liam/commit/a3c939a66e7f59597da19d53a13276890faa8ecc))
* **tui:** bordered input box, drop placeholder text ([#163](https://github.com/mgoodness/liam/issues/163)) ([c50e368](https://github.com/mgoodness/liam/commit/c50e368e3bc498dab5a0a53c35574a2aa979b1b5))
* **tui:** copy transcript text via click-drag selection and OSC-52 ([#150](https://github.com/mgoodness/liam/issues/150)) ([138a10f](https://github.com/mgoodness/liam/commit/138a10f8f29c6bc2a607d36d670d7144eb191a11))
* **tui:** implement /&lt;skill-name&gt; force-activation command ([#125](https://github.com/mgoodness/liam/issues/125)) ([0b6102f](https://github.com/mgoodness/liam/commit/0b6102f067e1d7e12b416e972f3ad26ee83263bf))
* **tui:** pad the turn-in-progress indicator with blank lines above and below ([#165](https://github.com/mgoodness/liam/issues/165)) ([23b24b3](https://github.com/mgoodness/liam/commit/23b24b3a28a788ab45cd022823a5ee90be608b61))
* **tui:** render mention/slash popups as floating bordered dialogs ([#147](https://github.com/mgoodness/liam/issues/147)) ([b376cb2](https://github.com/mgoodness/liam/commit/b376cb25ffc6816df115628b89aa4782f948622d))
* **tui:** render slash commands and /skills as an aligned, truncated table ([#157](https://github.com/mgoodness/liam/issues/157)) ([c64f2d2](https://github.com/mgoodness/liam/commit/c64f2d25163c24dd3801888d402ae5cd3b13d2e8))
* **tui:** show an animated turn-in-progress indicator above the input ([#154](https://github.com/mgoodness/liam/issues/154)) ([cd41d71](https://github.com/mgoodness/liam/commit/cd41d71dde43e10839a87852b45b1f0dc1604307))
* **tui:** suggest slash commands via a fuzzy popup as you type ([#140](https://github.com/mgoodness/liam/issues/140)) ([848c9e4](https://github.com/mgoodness/liam/commit/848c9e497f9baa4a97156c7c2a436cf0b084de78))


### Bug Fixes

* **ci:** run the test matrix on release-please branches too ([#173](https://github.com/mgoodness/liam/issues/173)) ([dedf0ac](https://github.com/mgoodness/liam/commit/dedf0ac4f3ac1f3de907d18e8a2f2fcca428188f))
* **cmd/liam:** stop logging find/grep searcher choice to stderr ([#129](https://github.com/mgoodness/liam/issues/129)) ([99e4742](https://github.com/mgoodness/liam/commit/99e474257495873df13ce8c7ffe74a5fbab0dbac))
* **identity:** capitalize Liam in self-identification strings ([#168](https://github.com/mgoodness/liam/issues/168)) ([79dfa04](https://github.com/mgoodness/liam/commit/79dfa048c70d1ff1db685a0fd51d4699224fc77f))
* **statusline:** render the status block below the input line ([#119](https://github.com/mgoodness/liam/issues/119)) ([e651f76](https://github.com/mgoodness/liam/commit/e651f76abc2f94b1e1897bd669524961af0018d1))
* **tui:** blend popup dialog background with the main viewport ([#162](https://github.com/mgoodness/liam/issues/162)) ([7842192](https://github.com/mgoodness/liam/commit/7842192ec2888cbae685a2ab0e52ce9fcd08d24a))
* **tui:** print the full /&lt;skill-name&gt; command like any other input ([#135](https://github.com/mgoodness/liam/issues/135)) ([0f00734](https://github.com/mgoodness/liam/commit/0f007342af533319d1ceb295dfe1fae34e78d789))
* **tui:** render the status block below the input line ([#136](https://github.com/mgoodness/liam/issues/136)) ([71f85e0](https://github.com/mgoodness/liam/commit/71f85e0ac03f8b9af917e4d9c22aac735d7c4827))
* **tui:** resume auto-follow when a manual scroll reaches the bottom ([#131](https://github.com/mgoodness/liam/issues/131)) ([a8f7c6d](https://github.com/mgoodness/liam/commit/a8f7c6d7dbaf2ae8382172d5ebb87258cff77325))
* **tui:** send trailing text after /&lt;skill-name&gt; as the first turn ([#134](https://github.com/mgoodness/liam/issues/134)) ([e7988a5](https://github.com/mgoodness/liam/commit/e7988a58207531cc7b40e26535540a01ccecabf5))

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
