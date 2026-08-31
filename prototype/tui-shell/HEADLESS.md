# Headless mode output shape (not a TUI artifact — written description only)

For the same mocked interaction the TUI variants show (`liam -p "add error handling to the CSV parser"`):

```
$ liam -p "add error handling to the CSV parser"
I'll take a look at the parser first.
⚙ grep(pattern: "func ParseCSV", path: "internal/csv") → internal/csv/parser.go:14
Found it. Reading the file, then I'll add error handling around the row-splitting logic.
⚙ read(path: "internal/csv/parser.go") → 82 lines
Now applying the edit.
⚙ edit(path: "internal/csv/parser.go") → 6 lines changed
```
```
# stderr, one line per response:
# model: anthropic/claude-opus-4.7
```

- No TUI, no boxes/borders — plain text to stdout, colorized only if stdout is a TTY and `NO_COLOR` is unset (standard CLI convention).
- Assistant text streams to stdout as generated.
- Tool calls print as one line each, same `⚙ name(args) → result` shape as the TUI's inline rendering, just without color or a box — same plain-text convention established for `find`/`grep`/`web_search` (tickets #18/#19).
- The actually-used model note goes to **stderr** after each response (ticket #20), keeping stdout clean/scriptable.

**Gap surfaced by this prototype, not previously settled:** headless mode has no TTY to show an interactive permission prompt in — the "allow once / allow for session / deny" UX from ticket #22 is TUI-only by construction. Headless needs an explicit policy for a `prompt`-permission tool call: auto-deny by default (safe, matches CI expectations) with a clear stderr message telling the user to either name the command in `permissions` config or pass `--yolo`, rather than hanging forever waiting for input that can never arrive.
