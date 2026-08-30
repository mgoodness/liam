# Go options for detecting terminal light/dark mode

Research for [liam#9](https://github.com/mgoodness/liam/issues/9), feeding [liam#21](https://github.com/mgoodness/liam/issues/21) (Catppuccin theme auto-switching).

## Recommended path: Bubble Tea's built-in background query

Since liam is already committed to Bubble Tea, the most direct integration point is Bubble Tea v2's own color API, rather than reaching for termenv/lipgloss separately:

- `tea.RequestBackgroundColor() tea.Msg` — a `Cmd` a program returns (typically from `Init`) to ask the terminal for its background color. ([bubbletea/color.go](https://github.com/charmbracelet/bubbletea/blob/main/color.go))
- `tea.BackgroundColorMsg` — the response message, wrapping a `color.Color`, delivered to `Update`.
- `BackgroundColorMsg.IsDark() bool` — darkness classification, delegated to `uv.BackgroundColorEvent(e).IsDark()` from the `ultraviolet` library. ([bubbletea/color.go](https://github.com/charmbracelet/bubbletea/blob/main/color.go))

Usage shape: return `tea.RequestBackgroundColor` from `Init`, handle the `case tea.BackgroundColorMsg` branch in `Update`, call `msg.IsDark()` to pick Catppuccin Frappe (dark) vs. Latte (light). Bubble Tea's own `color.go` source has no comments on timeout or fallback-when-unsupported behavior for this path — it delegates to `ultraviolet` — so that behavior needs verifying directly against `ultraviolet` if precise timeout/fallback semantics matter for liam's startup latency budget.

## Underlying mechanism: OSC 11

Both Bubble Tea's background query and the standalone lipgloss/termenv APIs are built on the OSC 11 terminal escape sequence:

- **Query**: `ESC ] 11 ; ? (BEL | ESC \)` — asks the terminal to report its background color.
- **Response**: the terminal replies with `ESC ] 11 ; rgb:RRRR/GGGG/BBBB (BEL|ESC\)`, 16-bit-per-channel hex. ([OSC 11 reference, vtdn.dev](https://vtdn.dev/docs/osc/osc11/))
- Applications like Vim, Neovim, and tmux already rely on this exact mechanism to pick dark/light colorschemes.

## Standalone alternative: lipgloss / termenv

If liam ever needs background detection outside a running Bubble Tea program (e.g. a headless/non-interactive code path — relevant since liam's v1 scope includes a headless mode per the map's Notes), the standalone API is:

```go
hasDarkBG := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
```

Documented behavior ([pkg.go.dev/github.com/charmbracelet/lipgloss/v2](https://pkg.go.dev/github.com/charmbracelet/lipgloss/v2)):
- Takes `in`/`out` `term.File` args — typically stdin and stdout-or-stderr.
- **"By default, this function will return true if it encounters an error"** — i.e. it conservatively assumes a dark background on failure (non-TTY, timeout, unsupported terminal).
- The docs explicitly say: *"This is intended for use in standalone Lip Gloss only. In Bubble Tea, listen for `tea.BackgroundColorMsg` in your Update function."* — confirming the Bubble Tea path above is the intended one for liam's interactive TUI, with this standalone call reserved for the headless path.

Underneath, `termenv`'s implementation (`termenv_unix.go`, `muesli/termenv`) does the following, in order ([github.com/muesli/termenv](https://github.com/muesli/termenv)):
1. Checks the `COLORFGBG` env var first (format `fg;bg`); if set and parseable, uses the background index from it — no terminal round-trip needed.
2. Otherwise sends the OSC 11 query (`termStatusReport(11)`), followed by a cursor-position-request as a secondary fallback signal, and waits up to `OSCTimeout = 5 * time.Second` for a response.
3. Explicitly **refuses to query** under `screen`/`tmux`/dumb terminals, returning `ErrStatusReport` immediately — rationale given in the source: those multiplexers "can be connected to multiple terminals concurrently," so a single OSC 11 answer isn't meaningful.
4. If both the OSC query and `COLORFGBG` fail, defaults to `ANSIColor(0)` (black — i.e. dark).

## Practical implications for liam

- **Interactive TUI**: use `tea.RequestBackgroundColor`/`tea.BackgroundColorMsg` — it's already in the dependency liam is committing to, and is the API Bubble Tea's own docs point at.
- **Headless mode**: use `lipgloss.HasDarkBackground(os.Stdin, os.Stdout)` for the same detection outside the Elm-architecture loop.
- **tmux/screen users**: both paths inherit termenv's refusal to query inside multiplexers — expect a same-as-error fallback (dark) unless liam separately reads `COLORFGBG` or a config override. Worth surfacing as an explicit manual-override config option (ticket [liam#21](https://github.com/mgoodness/liam/issues/21)) since a meaningful fraction of terminal users run tmux.
- **OS-level detection** (macOS `AppleInterfaceStyle`, Windows `AppsUseLightTheme` registry value, GNOME/KDE `color-scheme` settings) was not needed as a fallback path here — termenv's `COLORFGBG`-then-OSC-11-then-default-dark chain already covers the realistic terminal matrix (iTerm2, Terminal.app, Alacritty, Windows Terminal, and tmux/screen as the refusal case) without shelling out to OS-specific APIs. Not further researched since none of the primary sources above reference OS-level dark-mode APIs as part of standard terminal-background detection.
