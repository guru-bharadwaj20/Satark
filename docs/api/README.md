# Satark public API

Satark's REST API lets other apps (a residents' association portal, a college helpdesk bot, a browser extension) check a message, link or QR code before a user acts on it.

- **Reference:** [`index.html`](index.html) renders [`openapi.yaml`](openapi.yaml) with Redoc. Serve this folder from any static host, or open it locally with `npx serve docs/api`.
- **Source of truth:** `api/openapi.yaml`. Don't edit `docs/api/openapi.yaml` by hand; run `scripts/publish-api-docs.sh`. CI runs it with `--check`.
- **Internal:** [`reputation.openapi.yaml`](reputation.openapi.yaml) documents the `reputation` service, which is only reachable inside the cluster (the gateway and the bot call it).

## Authentication

Every request needs an API key in the `X-API-Key` header. Keys look like `sk_live_` followed by 32 random characters. See [API keys](api-keys.md) for how to request one.

```bash
curl -s https://api.satark.example/v1/check/text \
  -H "X-API-Key: $SATARK_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"text":"Dear customer, your account will be blocked today. Update KYC at kyc-update.example"}'
```

## Rate limits

Each key has a per-minute and per-day quota (default 30/min and 1,000/day). A `429` response has a `Retry-After` header. Back off and retry after that many seconds.

## Using results responsibly

- Results are a **risk assessment, not a guarantee**. A `low` result means no warning signs were found. Never show it to users as "safe" or "100% safe".
- Never label a person as a scammer based on a result or on community reports.
- Don't send personal data you don't need. Satark redacts phone numbers, UPI IDs, account numbers and OTPs before any LLM sees the text, but the less you send the better.
- Show users the official channels: call **1930** if money was lost, report at **cybercrime.gov.in**, and report fraud calls and messages on **Chakshu** (sancharsaathi.gov.in).

## Errors

Errors are JSON: `{"error": "human-readable message"}` with a standard HTTP status (`400` invalid input, `401` missing or invalid key, `429` rate limited, `5xx` server error; retry with backoff).
