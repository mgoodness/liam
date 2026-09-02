# Copy from the transcript via in-app OSC-52 selection, not a mouse-mode toggle

Mouse-wheel scrolling (#96) put the transcript viewport into `MouseModeCellMotion`, which means the terminal routes all mouse events to liam instead of doing native click-drag text selection — reported as "can't copy text from the transcript" ([issue #142](https://github.com/mgoodness/liam/issues/142)). We chose to build real click-drag selection inside liam that copies via OSC-52 (`tea.SetClipboard`) on release, rather than adding a keybinding to toggle mouse mode off and fall back to the terminal's own selection. OSC-52 keeps scroll-wheel and selection both working without the user ever needing to context-switch modes, and Bubble Tea already ships the primitive. The one accepted gap: OSC-52 isn't universal (some terminals don't support it, and [herdrdev/herdr#2399](https://github.com/herdrdev/herdr/issues/2399) is an open bug where herdr silently drops OSC-52 over SSH/WSL topologies) — for those cases the terminal's own modifier-click bypass (documented in the README) remains the fallback.

## Considered Options

A runtime keybinding to toggle `MouseModeCellMotion` on and off, falling back to the terminal's native selection while toggled off. Rejected: forces the user to remember a mode switch mid-task for something that should feel as immediate as selecting text anywhere else, and scroll-wheel would stop working while selection mode is active.

Docs-only fix (README note about the modifier-click bypass, no code change). Rejected as the sole fix: it works today with zero engineering cost, but doesn't address the underlying friction that prompted the issue — kept as a documented fallback alongside the OSC-52 feature, not instead of it.
