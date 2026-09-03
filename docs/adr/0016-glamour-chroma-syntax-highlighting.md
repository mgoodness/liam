# Adopt `glamour` for transcript markdown rendering, `chroma` directly for diffs/file views

liam's transcript renders zero markdown today — the model's own markdown-formatted responses show as raw text, literal `**`/backtick marks included. liam adopts `charmbracelet/glamour` (#152) to render full markdown (headers, bold, lists, syntax-highlighted code fences) in one pass, plus `chroma` directly for the non-markdown code surfaces `read`/`edit`'s diff output (#126) already produces — matching `charmbracelet/crush`'s own confirmed dependency pair (`glamour` + `chroma` both direct in its `go.mod`).

This is a real, load-bearing dependency addition — but a low-risk one, in the same spirit as the Bubbletea/Lipgloss/Bubbles exceptions `CODING_STANDARDS.md` already lists rather than a departure from it: `glamour` is the same vendor's sibling library, actively maintained, already independently validated by two peers (pi-go, crush) for exactly this purpose. The decisive fact tipping this from "maybe" to "yes": chroma (glamour's internal highlighter) ships official built-in styles named exactly `catppuccin-frappe`/`catppuccin-latte`, matching liam's own two theme names precisely — no custom color-mapping work to author, just select the style by name off the existing `theme.mode` resolution.

Scope grew beyond the ticket's literal title ("themed syntax highlighting") to include full markdown rendering, because syntax-highlighting a code fence doesn't mean much while liam still renders everything else around it as raw text with literal markdown syntax visible — the narrower ask doesn't stand on its own without the broader fix underneath it.

Headless mode is explicitly excluded: it stays plain text, no ANSI, matching liam's established scriptable/pipeable tool-output convention — color codes there risk breaking downstream parsing for a mode whose whole purpose is machine-consumable output. LSP integration (semantic highlighting, go-to-definition, diagnostics) is explicitly out of scope too — genuinely different from lexical/grammar-based syntax highlighting, despite the ticket's own "maybe LSP support?" aside.

## Considered Options

Narrow scope: adopt `chroma` only, for code-fence highlighting specifically, leaving the rest of markdown as raw text. Rejected: doesn't actually solve the underlying problem — highlighting a code fence while everything around it still shows literal `**`/backtick syntax is an inconsistent, half-fixed transcript.

Author a custom Catppuccin style file for `glamour`/`chroma` rather than using the built-in ones. Rejected: unnecessary — chroma's built-in `catppuccin-frappe`/`catppuccin-latte` styles are an exact name-and-color match for liam's own themes already.

Extend headless mode with colored output too. Rejected: breaks the scriptable/pipeable contract that mode exists for.

## Consequences

liam takes on `glamour`, `chroma`, `goldmark`, and `bluemonday` as new transitive dependencies — a real addition to the dependency footprint, though a narrow, same-vendor-adjacent one. The transcript's rendering pipeline gains a second code-highlighting call site (`chroma` directly, for diffs/file views) alongside `glamour`'s own internal use of it — both keyed off the same style names, so they can't visually drift apart by construction.
