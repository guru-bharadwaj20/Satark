# Demo video script (3 minutes)

**Goal:** someone who has never heard of Satark understands in three minutes what it does, trusts it, and knows how to start. Secondary audience: recruiters and engineers, who get the last 40 seconds.

**Format:** 1080p screen recording (phone mirrored with `scrcpy`, plus desktop), voice-over, captions burned in. Record in English; add Hindi and Kannada subtitles afterwards.

## Before recording

- [ ] Use the **dev** environment with a fresh test Telegram account. No real contacts, names or numbers on screen.
- [ ] Seed 3 dev reports for `9000000001` so the lookup shows a result.
- [ ] Prepare the inputs in a notes app:
  - KYC message: _"Dear customer your SBI account will be blocked today. Update KYC at sbi-kyc-update.example"_
  - UPI QR for `upi://pay?pa=xy.test@examplebank&am=4999&tn=Refund`
  - Job message: _"Earn ₹3000/day liking YouTube videos. Contact on Telegram"_
- [ ] Phone on Do Not Disturb, battery and clock hidden (`scrcpy --no-control` + crop).
- [ ] Grafana dashboard and `kubectl` terminal open on the desktop.

## Script

| Time | Screen | Voice-over |
|---|---|---|
| 0:00–0:15 | News headlines about UPI and KYC fraud (blurred, no logos), then a phone receiving the KYC SMS | "Every day, people in India get messages like this. It looks official, it's urgent, and you have about thirty seconds to decide." |
| 0:15–0:25 | Satark logo + "Check before you click, pay or scan" | "Satark is a free, open-source assistant that helps you check before you act." |
| 0:25–0:55 | Forward the KYC SMS to the bot; verdict appears | "Forward the message to the Satark bot. In a few seconds it says this is high risk, and explains why: a new lookalike domain, an urgent account-block threat, and a request to update KYC by link. It tells you what to do: don't click, and banks never ask for this." |
| 0:55–1:20 | Share a screenshot of the QR from the gallery to the PWA via the Android share menu | "Got a QR code? Share it straight from your gallery. Satark decodes it without paying anything and shows it's a collect request: scanning it would take ₹4,999 out of your account, not put money in." |
| 1:20–1:40 | Switch to Hindi with `/language`, send the job message | "It works in Hindi and Kannada too, and recognises common scams like fake part-time jobs." |
| 1:40–2:05 | Tap Report; finish the flow; show the 1930 / cybercrime.gov.in / Chakshu steps and the copy block | "If you've been targeted, tap Report. Your report helps warn others, and Satark gives you the exact steps for 1930, cybercrime.gov.in and Chakshu, with the details ready to paste." |
| 2:05–2:20 | Lookup of the seeded number, showing masked value and "fewer than 10 independent people" | "Anyone can look up a number or UPI ID. Satark only shows something after several independent people report it, masks the number, and never calls anyone a scammer." |
| 2:20–2:45 | Desktop: architecture diagram, `kubectl get pods`, a link opening in the sandbox, Grafana | "Under the hood: Go and Python services on Kubernetes. Suspicious links open in a throwaway, isolated browser that never types anything. Personal data is removed before any AI sees the text. Everything is monitored, and there's an AI on-call assistant that reads our runbooks." |
| 2:45–3:00 | Bot link, QR to the bot, GitHub URL, disclaimer text | "Try it on Telegram, or install the web app. Satark gives a risk assessment, not a guarantee. If you've lost money, call 1930 immediately." |

## After recording

- [ ] Edit to 3:00 or less; captions on every line.
- [ ] Check every frame for real personal data before export.
- [ ] Export 1080p H.264; upload as unlisted first, review, then public.
- [ ] Cut a 30 s vertical version (0:15–0:55 + end card) for WhatsApp status and Instagram.
- [ ] Use 0:25–0:55 for `docs/media/demo.gif` (see `docs/README-media.md`).
