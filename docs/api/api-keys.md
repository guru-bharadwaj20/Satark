# API keys for third-party clients

Satark's public API is free for non-commercial projects that help people avoid scams: residents' association portals, college helpdesks, NGO tools, browser extensions.

## Requesting a key

Open a GitHub discussion in the **API keys** category (or contact the maintainer privately) with:

1. The project name and a link to it.
2. What you'll check (text, links, QR codes) and roughly how many checks a day.
3. A contact email for quota and security notices.
4. Confirmation that you'll follow the [responsible-use rules](README.md#using-results-responsibly): never present a result as "100% safe" and never label a person a scammer.

Turnaround is usually within a week. Default quota is 30 requests per minute and 1,000 per day.

## Issuing a key (maintainers)

```bash
# Dry run: print the SQL without touching the database
DRY_RUN=1 scripts/api-key.sh issue "RWA Portal" admin@rwa.example

# Issue for real (needs kubectl access to the satark-data namespace)
scripts/api-key.sh issue "RWA Portal" admin@rwa.example 30 1000
```

The plaintext key (`sk_live_...`) is printed **once** to stderr. Only its SHA-256 hash and a 12-character prefix are stored. Send the key privately, for example as an encrypted email or a one-time secret link. Never paste it into an issue, chat or ticket.

```bash
scripts/api-key.sh list                    # prefixes, quotas, state
scripts/api-key.sh revoke sk_live_AbCd     # takes effect within 60s
```

## Rotation and revocation

- Revoke immediately if a key appears in a public repo, app bundle or log. Issue a replacement and tell the client.
- Keys don't expire, but unused keys (no requests for 180 days) are revoked by the quarterly review.
- Keys must only be used server-side. A key shipped in a mobile app or browser extension will be extracted; those clients should proxy through their own backend.

## Verify

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://api.satark.example/v1/check/text \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' -d '{"text":"test"}'   # expect 200
# after revocation (and the 60s cache), expect 401
```
