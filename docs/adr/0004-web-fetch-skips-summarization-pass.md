# `web_fetch` skips a secondary summarization pass

liam's `web_fetch` tool (v0.2) does a raw HTTP GET and converts HTML to readable text, then applies the same tool-output truncation cap every other tool already uses (~50KB) — it does not run the fetched content through a second, cheaper model call to extract/summarize relative to the calling prompt, the way Claude Code's own `WebFetch` does.

The reasoning was resource-driven, not principled: Claude Code's `WebFetch` can afford a summarization pass because Anthropic's model family includes a small, cheap tier to spend on it. liam has exactly one configured model (whatever Copilot model the user picked) — a "summarization pass" would mean a second full-price call to that same model, not a cheap one. Given that, and that the existing truncation cap already prevents a huge page from flooding context (just less intelligently — a hard byte cutoff instead of an extractive summary), we skipped it for v0.2.

This is worth revisiting if it turns out truncation alone produces noticeably worse results than extraction would — e.g. if a long page's relevant section gets cut off before the truncation boundary. Nothing here is architecturally locked in: the tool could grow a second Provider-mediated pass later without changing its external contract (still one string result).

## Considered Options

- **Raw fetch + truncation cap** (chosen) — no second model call, reuses infrastructure every other tool already has. Cruder than extraction for very long pages.
- **Raw fetch + secondary summarization call** — better signal-to-noise for long pages, but doubles the cost of every `web_fetch` invocation (a second full-price call to the same model) for a capability liam doesn't have a cheap tier to spend on cheaply the way Claude Code does.
