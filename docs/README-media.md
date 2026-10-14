# README media: what to capture

The README will show a few screenshots and a short demo GIF. They have to be captured by hand from a running instance. Put the files in `docs/media/` with exactly these names, then add them to the README (the coordinator owns README edits).

## Rules for every capture

- **No real personal data.** Use only the synthetic examples below. Phone numbers look like `98XXXXXX21`, UPI IDs like `xy***@examplebank`, domains end in `.example`.
- Crop out your own Telegram name, avatar, phone status bar and notifications.
- Light theme for screenshots (the GIF can be either). Width around 1080 px for phone shots, 1600 px for desktop.
- PNG for screenshots, optimised: `pngquant --quality 70-90 --ext .png --force docs/media/*.png`.
- GIF under 5 MB: record at 15 fps, then `gifski --fps 15 --width 480 -o docs/media/demo.gif frames/*.png`.

## Shot list

| File | What it shows | How to set it up |
|---|---|---|
| `docs/media/bot-check-high.png` | Bot replying "High risk" to a forwarded KYC message, with reasons and the 1930 line | Forward: _"Dear customer your SBI account will be blocked today. Update KYC at sbi-kyc-update.example"_ |
| `docs/media/bot-check-low.png` | A low-risk result that says "No warning signs found... doesn't guarantee it's safe" | Send a normal message like _"Lunch at 1?"_ with `https://example.com` |
| `docs/media/bot-qr-upi.png` | QR scan of a UPI collect request, showing the payee and amount warning | Generate a QR for `upi://pay?pa=xy.test@examplebank&am=4999&tn=Refund` |
| `docs/media/bot-report-flow.png` | End of the report flow with 1930 / cybercrime.gov.in / Chakshu steps and copy block | `/report` → UPI ID → `xy.test@examplebank` → UPI collect → Yes → Submit |
| `docs/media/bot-lookup.png` | Lookup showing a masked subject and "fewer than 10 independent people" | Seed 3 test reports in dev for `9000000001`, then look it up |
| `docs/media/pwa-home.png` | PWA home on a phone | Open the PWA on Android Chrome |
| `docs/media/pwa-share-menu.png` | Satark in the Android share sheet | Install the PWA, long-press a message in Messages → Share |
| `docs/media/pwa-result.png` | PWA showing a result card | Paste the KYC message from the first row |
| `docs/media/grafana-overview.png` | Platform dashboard (request rate, latency, error budget) | Grafana → Satark Overview, last 24h |
| `docs/media/architecture.png` | Optional: export of the container diagram from `docs/architecture.md` | `npx @mermaid-js/mermaid-cli -i diagram.mmd -o docs/media/architecture.png` |
| `docs/media/demo.gif` | 20–30 s loop: forward message → high-risk verdict → tap Report → copy details | Follow the first 30 s of `docs/launch/demo-video-script.md` |

## Where they go in the README

- `demo.gif` directly under the title and one-line pitch.
- `bot-check-high.png`, `bot-qr-upi.png` and `pwa-share-menu.png` side by side in the "What it does" section.
- `grafana-overview.png` in the operations/tech section.

Placeholders in `docs/media/` (`.gitkeep`) keep the folder in the repo until the real files land.
