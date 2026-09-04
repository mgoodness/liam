# Live theme re-detection, take two: DEC mode 2031 color-scheme push, alongside (not instead of) focus events

ADR-0010 chose terminal-focus-triggered OSC-11 re-query for #103's live theme re-detection, explicitly rejecting OS-level appearance APIs partly because "none are cross-platform the way OSC 11 + a focus event is." That claim doesn't fully hold up: DEC private mode 2031 ("color scheme reporting," an emerging VT extension originating in the Contour terminal) is a real, standardized *terminal protocol* — not an OS API — that a supporting terminal uses to proactively push `CSI ?997;1n` (dark) or `CSI ?997;2n` (light) the moment the OS-level color scheme actually changes, with no polling and no dependency on focus. ADR-0010 itself already gestured at this ("herdr... DEC mode 2031 push notifications where the host supports it") without following through.

#203 asked to revisit this. Grilling settled on adding it as a **second, independent** re-detection path alongside #103's focus-triggered one, not a replacement — see Considered Options for why "replace" was rejected even though the maintainer's own environment (herdr, Ghostty-derived) supports mode 2031.

## Mechanism

liam's dependency chain already has everything needed, verified by direct inspection rather than assumed:

- **Enabling it**: `tea.Raw(ansi.SetModeLightDark)` — i.e. `\x1b[?2031h`. `tea.Raw` is Bubble Tea's own documented, exported escape hatch ("intended for advanced use cases where you need to query the terminal or send escape sequences directly"), routed through the same mutex-protected output path as every other terminal query (`Program.execute`/`Program.flush`). No hack, no upstream contribution needed.
- **Receiving it**: liam's pinned `ultraviolet` dependency (currently indirect) already decodes the push into `uv.DarkColorSchemeEvent{}`/`uv.LightColorSchemeEvent{}` (`decoder.go`). Bubble Tea v2.0.9's `translateInputEvent` has no case for either type, but its `default` branch returns the raw event unchanged — so these already reach `Model.Update` as `tea.Msg` today, typeable via `case uv.DarkColorSchemeEvent:`/`case uv.LightColorSchemeEvent:` once `ultraviolet` is promoted to a direct import.
- **No self-poisoning risk**: unlike #103's OSC-11 GET re-query (see docs/adr/0010's addendum for that whole saga), these are direct push signals carrying their own definitive dark/light answer — no query/response pair for liam's own background-painting to poison.
- **Cleanup**: `tea.Raw`, unlike a `View()` field (e.g. `ReportFocus`), gets no automatic reset-on-exit from Bubble Tea's renderer. liam sends `tea.Raw(ansi.ResetModeLightDark)` on quit so it doesn't leave mode 2031 enabled in the terminal past its own session — the same hygiene `View()`-modeled modes get for free.
- **Gating**: identical to #103 — only active under `theme.mode: "auto"`; an explicit `dark`/`light` override skips enabling it entirely, same as it already skips the focus-triggered path.

## Verification

Terminal-protocol support was checked by direct source inspection and upstream issue tracking, not assumed:

| Terminal | Support |
|---|---|
| Ghostty | ✓ (v1.0.0+) |
| Kitty | ✓ |
| tmux | ✓ (3.6+, PR #4353) — notably, tmux/screen is exactly where #103's focus-triggered path is a documented no-op (#183), so this path gives tmux users live re-detection the other mechanism structurally can't |
| foot | ✓ (1.23.0+) |
| contour | ✓ (originating implementation) |
| iTerm2 | ✓ (nightly builds) |
| VTE (GNOME Terminal, etc.) | ✓ |
| herdr | ✓ — confirmed by direct raw-escape-sequence testing (enabled mode 2031, toggled the real OS appearance with no focus event, captured the raw push bytes on the pty: `\x1b[?997;1n`) |
| WezTerm | ✗ (open issue, wezterm/wezterm#6454) |
| Alacritty, Windows Terminal, Terminal.app | unconfirmed, likely unsupported |

Real but partial — "meaningful and growing," not universal. This is exactly why it's additive rather than a replacement.

## Considered Options

**Replace #103's focus-triggered mechanism entirely.** Initially the maintainer's preference, on the reasoning that their own environment (herdr/Ghostty) already supports mode 2031. Rejected on reflection: the focus-triggered path is already built, tested, and shipped (#201/#205) — keeping it costs nothing further, while removing it would regress every terminal without mode 2031 support (WezTerm, Alacritty, Windows Terminal, tmux <3.6, and anything unconfirmed) down to startup-only detection, for zero benefit to the maintainer's own actual usage. Coexisting gives the maintainer's own environment the identical experience (mode 2031 fires and wins) while not regressing anyone else.

**Wait for upstream Bubble Tea to add first-class support** (a `View.ReportColorScheme` field and wrapped `ColorSchemeMsg` types, mirroring `ReportFocus`/`FocusMsg` exactly). Rejected: `tea.Raw` already provides a safe, documented, synchronized path today — no reason to block on an upstream contribution that may never land, when the underlying primitive (`ansi.ModeLightDark` in `charmbracelet/x/ansi`, already a transitive dependency) is already available.

**Poll `ansi.RequestModeLightDark` on a timer instead of relying on the push.** Never seriously considered — this is the exact polling ADR-0010 already rejected for #103, for the same reasons (no comparable precedent, round-trip cost for no confirmed value), and the push mechanism exists specifically to avoid needing it.

## Consequences

liam gains a second, independent live-re-detection path that — unlike #103's — works inside tmux and doesn't depend on focus at all, on the (currently partial, growing) set of terminals that implement DEC mode 2031. Both paths are gated identically under `theme.mode: "auto"` and can fire independently; whichever resolves first simply sets `m.pal`, with no conflict between them since mode 2031's push carries its own definitive answer rather than triggering an OSC-11 round trip. Requires promoting `ultraviolet` from an indirect to a direct dependency. Terminals without mode 2031 support see no change at all — #103's focus-triggered mechanism remains their only (and already-correct) path.
