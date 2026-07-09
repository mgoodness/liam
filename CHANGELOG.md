# Changelog

## 1.0.0 (2026-07-09)


### Features

* **agent:** add agent loop ([#5](https://github.com/mgoodness/liam/issues/5)) ([c8cbe0a](https://github.com/mgoodness/liam/commit/c8cbe0ae14c266eb90d68f28b37b98ff38ebd435))
* **cmd:** add liam REPL and CLI wiring ([#6](https://github.com/mgoodness/liam/issues/6)) ([2cc6797](https://github.com/mgoodness/liam/commit/2cc6797537026e0a4a360cdd752d9b80351cbdbe))
* **provider:** add Copilot device-flow login and credential storage ([#8](https://github.com/mgoodness/liam/issues/8)) ([9754e41](https://github.com/mgoodness/liam/commit/9754e41c6ae394a89de4128dfebb81557469fb99)), closes [#3](https://github.com/mgoodness/liam/issues/3)
* **provider:** add Provider interface and Copilot chat completion ([#4](https://github.com/mgoodness/liam/issues/4)) ([e47b578](https://github.com/mgoodness/liam/commit/e47b578a80c202eb2a7bcddbd2efb4d6b20ec231))


### Bug Fixes

* **agent:** match tool.Call's unknown-tool contract, avoid history aliasing ([f957333](https://github.com/mgoodness/liam/commit/f9573335b5bee617eed21dddcfbd3baa040625e2))
* **cmd:** propagate genuine read errors from nextMessage instead of swallowing them ([c0fa04d](https://github.com/mgoodness/liam/commit/c0fa04d276f792583f8c8cbd2b61f09c41c8d2fb))
* **provider:** send required editor-identity headers on Copilot token exchange ([d96fd28](https://github.com/mgoodness/liam/commit/d96fd283e5a8fe12dc4c7cd0e86463040a7a3d14))
