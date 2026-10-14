# Privacy policy

_Last updated: October 2026_

Satark helps you check suspicious messages, links and QR codes. This policy explains what we collect, why, and how to delete it. It applies to the Satark Telegram bot, the Satark web app (PWA) and the Satark API. Satark is an open-source, non-commercial project; it is not a government service.

## What we collect

| What | Why | How long |
|---|---|---|
| Your Telegram user ID and language choice | To reply to you in your language | Until you send `/forget` |
| Messages, links and QR images you send for a check | To check them. Phone numbers, UPI IDs, account numbers, OTPs and similar details are removed **before** any text is sent to an AI model | Not stored after the check |
| Scam reports you file (the number, UPI ID or website, and the scam type) | To warn other people once several people report the same thing | 365 days, or until you send `/forget` |
| A delisting request and the optional contact you give | To review the request and reply | Contact: 30 days after the decision. Request: 365 days |
| Technical logs (time, error codes; no message content) | To keep the service running | 14 days |

Reports are stored as one-way hashes. We cannot see who reported what, and other people only ever see a masked value (for example `98XXXXXX21` or `xy***@examplebank`) and a rough count, never your identity.

## What we don't do

- We don't sell or rent data, show ads, or build profiles.
- We don't ask for your OTP, UPI PIN, passwords or bank details. Never send them to anyone, including us.
- Links are opened in an isolated sandbox that never fills in forms or enters data.
- We never call any person a "scammer". Community data says how many people reported something, nothing more.

## Who processes data

- **Hosting:** our own servers and a cloud provider in India or nearby regions.
- **AI model provider:** receives only redacted text, to explain risks in plain language. It is not used to train their models under our agreement settings.
- **Telegram:** carries messages between you and the bot under Telegram's own privacy policy.

## Your rights

Under India's Digital Personal Data Protection Act, 2023 you can ask to access, correct or erase your data, and to withdraw consent.

- **Delete everything:** send `/forget` to the bot. Your settings, history and reports are deleted. Copies in backups age out within 30 days.
- **Ask a question or complain:** contact the maintainer privately through GitHub (see `CONTRIBUTING.md`). We aim to reply within 7 days.
- **Number, UPI ID or website wrongly reported?** Request a review; see `docs/sops/SOP-07-delisting-requests.md`.

## Children

Satark is not meant for children under 18 without a parent or guardian.

## Changes

We'll post changes here and in the bot's `/privacy` message. Significant changes will be announced in the bot before they apply.

---

See also the [disclaimer](disclaimer.md).
