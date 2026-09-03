# Theme re-detection: focus-event-triggered OSC-11 re-query, not polling

liam's theme detection (OSC 11 via `tea.RequestBackgroundColor()`) only ever fires once, at startup. #103 originally proposed fixing that with continuous polling — rejected initially (no comparable TUI polls; OSC 11 is refused entirely inside tmux/screen; round-trip cost for no confirmed value). That initial rejection turned out to be wrong once directly comparable prior art was actually checked: `claude` (Anthropic's own coding-agent CLI) does solve this, live, in production — direct binary inspection confirmed it re-issues the same OSC-11 query not on a timer, but on **terminal focus-regained**, using xterm's standard focus-reporting mode (`DECSET 1004`). Zero cost while idle; fires only at the moment a change would actually matter (the user returns focus after changing their OS/terminal theme elsewhere). Bubble Tea v2 (`v2.0.9`, already a liam dependency) exposes this natively as `tea.FocusMsg`/`tea.BlurMsg` — no new dependency needed.

liam adopts the same mechanism: on `tea.FocusMsg`, re-issue `tea.RequestBackgroundColor()`. This only activates under `theme.mode: "auto"` — an explicit `dark`/`light` override means the user has opted out of detection entirely, so there's nothing to re-query. Default-on within `auto` mode, no separate config gate.

tmux/screen support is explicitly deferred to a fast-follow, not bundled here. Claude Code separately solves that case too (a DCS-passthrough wrapper forwarding the query through the multiplexer to the real outer terminal), but replicating it means not routing through `lipgloss.HasDarkBackground`/termenv's existing blanket-refusal behavior — a real, separate technical lift. Without it, focus-triggered re-detection simply no-ops inside tmux/screen, matching today's existing behavior there (no regression, not a new blind spot).

Separately, herdr (used extensively for liam's own development) was checked given its known OSC-52 passthrough bug ([herdrdev/herdr#2399](https://github.com/herdrdev/herdr/issues/2399)) — herdr is architecturally unrelated to that risk here: it's built on `libghostty-vt` with dedicated, actively-maintained theme infrastructure (its own OSC 10/11 queries against the real host terminal, DEC mode 2031 push notifications where the host supports it), not a passthrough hack.

## Considered Options

Continuous polling (#103's original proposal). Rejected: no comparable TUI does this, meaningful round-trip cost, and — the decisive fact — a better mechanism exists and is already proven in production.

OS-level appearance APIs (macOS `AppleInterfaceThemeChangedNotification`, Windows registry change notification, Linux desktop portal `SettingChanged`). Rejected: genuinely event-driven where supported, but answers a different question (OS-wide preference) than "what is this actual terminal's background," which can diverge if a user configures terminal colors independent of their OS theme — and none are cross-platform the way OSC 11 + a focus event is.

Wontfix (the initially considered recommendation, before checking Claude Code directly). Rejected in hindsight: based on an incomplete prior-art sweep that missed the one directly comparable tool that actually solves this well.

## Consequences

liam's theme detection now depends on Bubble Tea's focus-reporting mode being enabled and correctly forwarded by whatever terminal/multiplexer liam runs inside — a dependency the original once-at-startup detection didn't have. tmux/screen users get no live re-detection until the fast-follow DCS-passthrough work lands, but also no regression from today.
