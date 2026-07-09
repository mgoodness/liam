# Agent loop progress via a callback struct, not channels or streaming

v0.1's `agent.Run` executes the entire loop silently: intermediate assistant text (produced alongside tool calls in the same response) is stored in history but never shown, and tool calls themselves are invisible until the loop finally returns tool-call-free text. For v0.2, `agent.Run` gains a `Callbacks` parameter — `OnText`, `OnToolCall`, `OnToolResult` — invoked synchronously at the point each thing happens, wired by the REPL to plain `stdout` prints. `OnToolResult` receives the same (already-truncated) result that gets fed back to the model, so what the user sees matches the model's own view.

This is a real API change to `agent.Run`, made without any of it becoming a permission/confirmation gate — YOLO mode is unaffected; this is purely visibility, decided before implementation because the shape (a callback struct vs. channels vs. an `io.Writer`) affects both the agent loop's signature and how tests exercise it.

## Considered Options

- **Channels / an event stream** — would let the REPL consume events concurrently, but liam's loop is single-goroutine and synchronous throughout; introducing channels here for a purely local, in-process notification is unneeded concurrency machinery.
- **An `io.Writer` passed to `agent.Run`** — simplest signature, but couples the agent loop to a specific text format up front, and a test would have to parse printed output to assert on what happened rather than inspecting structured calls.
- **A `Callbacks` struct of nil-able func fields** (chosen) — synchronous, no concurrency needed, and a test can pass a `Callbacks` that just appends to a slice, asserting on structured calls rather than parsed text. The REPL is the one place that turns these into printed output.

Per-tool call summaries (`bash` shows the command, `read`/`write`/`edit` show the path, etc.) live as an optional `Summarize` field on `tool.Tool`, colocated with each tool's own implementation, rather than teaching the display layer about every tool by name — a tool with no summarizer just shows its name.
