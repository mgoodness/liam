# Use GitHub Copilot Chat's internal API via device-flow OAuth, not GitHub Models

We want the harness to run on the user's existing Copilot subscription, not a separate quota. GitHub Copilot has no official third-party chat-completions API, but the internal endpoint IDE extensions use (`api.githubcopilot.com`) is reachable via the same device-flow OAuth exchange those extensions use: a GitHub OAuth token traded for a short-lived Copilot token. We chose this unofficial-but-well-trodden path over the official GitHub Models API (`models.github.ai`) because Models is a separate product with its own billing/quota — it wouldn't actually be "your Copilot subscription."

## Considered Options

- **GitHub Models** — official, public, PAT-authenticated. Rejected: different product, different quota, doesn't use the Copilot subscription itself.
- **Copilot Chat via device flow** (chosen) — unofficial/reverse-engineered, but the same mechanism VS Code, Neovim, and other third-party Copilot clients rely on.

## Consequences

The masquerade isn't limited to the OAuth `client_id` — every request to a `*.githubcopilot.com` endpoint (token exchange, chat completions) must also present `Editor-Version`/`Editor-Plugin-Version`/`User-Agent` headers identifying a recognized editor client, or GitHub 403s with "Please only use approved clients for Copilot" regardless of whether the token itself is valid. This bit us once already (the token-exchange endpoint was missing these while the chat-completions endpoint already had them) — see the `provider/auth.go` header constants.
