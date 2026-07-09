# Defer a TUI input layer to v0.2 rather than patching line-based input twice

v0.1 ships with the REPL's original input model unchanged: a message submits on a blank line, full stop — no paste detection, no manual multi-line continuation, no Shift+Enter. While closing out v0.1, we considered two narrower stdlib-only fixes for this (a `bufio.Reader.Buffered()` paste heuristic, and a trailing-backslash manual-continuation convention) but chose to build neither.

Shift+Enter — the behavior actually wanted — isn't observable from canonical-mode line reading at all: most terminals send the same byte for Enter and Shift+Enter. Seeing the difference requires raw terminal mode, a keyboard-reporting protocol (Kitty's protocol or xterm's `modifyOtherKeys`), a new dependency (`golang.org/x/term`), and a fallback path for the many terminals that don't support either protocol — which is to say, it requires building the terminal input/rendering layer that the original v0.1 spec (issue #1) deliberately scoped out ("no TUI rendering ... plain sequential stdout only"). Shipping the smaller stdlib heuristics now would mean solving this problem twice: once cheaply and partially, then again properly for Shift+Enter in v0.2.

## Considered Options

- **`bufio.Reader.Buffered()` paste heuristic** — pure stdlib, submits as soon as nothing more is already buffered. Handles single-line input and most pastes correctly, but a paste with no trailing newline followed by more manual typing can split into two messages.
- **Trailing-backslash continuation** (shell/Python convention), combined with the above — lets manual multi-line typing work, at the cost of misinterpreting a literal trailing `\` in pasted code (a Python/shell continuation, a Windows path) as a request to keep accumulating.
- **Defer to v0.2** (chosen) — ship v0.1 with blank-line-only input; build a proper raw-mode input layer (with keyboard-protocol detection and a fallback) as its own effort, addressing paste, manual multi-line, and Shift+Enter together instead of in two passes.
