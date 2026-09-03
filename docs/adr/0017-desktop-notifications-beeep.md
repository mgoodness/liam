# Desktop notifications: `beeep`, gated by existing focus-tracking, local-machine-only for v1

liam sends a desktop notification when the Agent loop's current invocation concludes (the same moment #102's `agentDone` hook point fires — a built-in harness behavior running alongside that signal, not implemented as a Hook itself), but only when the terminal is unfocused — reusing `tea.FocusMsg`/`tea.BlurMsg` tracking already built for #103/ADR-0010's theme re-detection, no new signal needed. Mechanism: `github.com/gen2brain/beeep`, matching `charmbracelet/crush`'s own confirmed choice (its README documents the identical gating logic verbatim: "only sent when the terminal window isn't focused and your terminal supports reporting the focus state"). Claude Code independently confirms the same trigger-and-gate pattern, though liam only has one trigger (turn completion) — its second trigger, a permission-prompt pause, doesn't map to anything liam has (ADR-0004: no built-in permission system).

`beeep` calls native OS notification APIs directly on whatever machine the process is running on — fine locally, useless over SSH (the notification fires on the remote host, not the user's actual desktop). Claude Code and crush both have a separate mechanism for that case (OSC-escape-sequence forwarding, translated to a real local notification by the terminal emulator, the same category of trick OSC-52 clipboard already relies on for liam per ADR-0006) — v1 doesn't build this; it's deferred to #196. Notably, native OS notification APIs don't route through terminal escape sequences at all, so — unlike OSC-11 theme detection or OSC-52 clipboard — tmux/screen aren't a problem for the local case regardless.

Config is `auto`/`disabled` only, not crush's fuller `auto|native|osc|bell|disabled` — the richer mode list only earns its keep once SSH/OSC support (#196) exists.

## Considered Options

Wiring this as a new Hook lifecycle point (or reusing `agentDone` itself) rather than a built-in behavior. Rejected: notification delivery is harness behavior with a config toggle, the same category as crush's own built-in feature — not something a user should need to configure a shell command for, the way `agentDone`'s audit/observability use case does.

Building SSH/OSC-forwarding support now, matching crush's full mode list. Rejected for v1: real, separate mechanism from `beeep`'s native-local path — deferred to #196 rather than blocking the higher-value, lower-effort local case behind it.

## Consequences

Notifications are silently useless for anyone running liam over SSH until #196 lands — an accepted, temporary gap, not a design flaw to work around locally. `beeep` becomes liam's first cross-platform native-notification dependency.
