# Exa search API surface & Go client availability

Research for [liam#3](https://github.com/mgoodness/liam/issues/3).

## Endpoint & auth

- `POST https://api.exa.ai/search`
- Auth via the `x-api-key` header, or an `Authorization: Bearer <key>` header.
- Source: [Exa API reference — Search](https://exa.ai/docs/reference/search)

## Request body (basic web search)

- `query` (string, required, min 1 char) — the only required field.
- `numResults` (int, 1–100, default 10)
- `type` (enum, default `auto`) — `instant`, `fast`, `auto`, `deep-lite`, `deep`, `deep-reasoning`
- `contents` — configuration object for extracting text, highlights, and summaries from each result
- `category` — restricts to a content type: `publication`, `news`, `company`, `people`

Source: [Exa API reference — Search](https://exa.ai/docs/reference/search)

## Response body

Top-level: a `results` array, plus `costDollars` (cost breakdown) and `searchTime` (ms).

Each entry in `results`:

| Field | Meaning |
|---|---|
| `title` | webpage heading |
| `url` | link to the resource |
| `publishedDate` | ISO 8601 estimated publish date |
| `author` | content creator, when available |
| `id` | temporary document id, usable with the `/contents` endpoint |
| `text` | full page content |
| `highlights` | extracted relevant snippets |
| `summary` | LLM-generated overview |

Source: [Exa API reference — Search](https://exa.ai/docs/reference/search)

## Go client availability

**No official Go SDK.** Exa Labs (github.com/exa-labs) maintains official SDKs for JavaScript ([exa-js](https://github.com/exa-labs/exa-js)) and Python ([exa-py](https://github.com/exa-labs/exa-py)) only. A third-party community client exists ([xmonader/exa-go](https://github.com/xmonader/exa-go)) but is unofficial and unverified for maintenance status.

**Implication for liam:** the Exa web-search tool needs a thin first-party HTTP wrapper — a small `net/http` client posting to `https://api.exa.ai/search` with the `x-api-key` header — rather than depending on an official or third-party Go SDK.
