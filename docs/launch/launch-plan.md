# Launch plan: family, college and residents' association groups

A soft launch to people we know, in three waves, so problems surface with a forgiving audience before anything public.

## Go / no-go checklist (before wave 1)

- [ ] Prod is green for 7 days: SLO dashboards, no open P1/P2 incidents.
- [ ] Load test at 10x expected wave-3 traffic passed (`tests/load`).
- [ ] Privacy policy and disclaimer live (`/privacy` in the bot, `/privacy.html` in the PWA).
- [ ] `/forget`, the report flow and the delisting request form work end to end in prod.
- [ ] Gateway rate limits on; Telegram bot flood control on.
- [ ] On-call rota for the first 2 weeks agreed; runbooks reviewed.
- [ ] Feedback buttons (👍 / 👎) on every result are recording (`satark_feedback_total`).
- [ ] KPI baseline captured (`scripts/kpi-report.sh`, see `kpis.md`).

## Waves

| Wave | Who | Size | When | Exit criteria to move on |
|---|---|---|---|---|
| 1 | Family groups (parents, aunts and uncles, grandparents) | ~30 people | Week 1 | No P1/P2 bug for 3 days; helpful rate ≥ 70% |
| 2 | College batch and department groups | ~200 people | Week 2 | p95 check latency < 5 s; no false "high risk" on banks' genuine SMS templates |
| 3 | Residents' association (RWA) groups, with the secretary's permission | ~500–1,000 people | Weeks 3–4 | Stable for 7 days; review ready for public launch |

Ask group admins first. Don't add the bot to groups; share the link and let people start it themselves.

## Messages (copy-paste)

### Family (English)

> Hi everyone! I've built a free tool called Satark that checks if a message, link or QR code looks like a scam, before you click or pay. Just forward the message to it on Telegram: <bot link>
> It doesn't keep your messages. It can be wrong, so if you're unsure, call me anyway. And remember: banks never ask for OTP or UPI PIN. If money is ever lost, call 1930 straight away.

### Family (Hindi)

> सबको नमस्ते! मैंने Satark नाम का एक मुफ़्त टूल बनाया है जो बताता है कि कोई मैसेज, लिंक या QR कोड धोखा तो नहीं है, क्लिक या पेमेंट करने से पहले। बस मैसेज को Telegram पर इसे फ़ॉरवर्ड करें: <bot link>
> यह आपके मैसेज सेव नहीं करता। यह गलत भी हो सकता है, इसलिए शक हो तो मुझे फ़ोन कर लें। बैंक कभी OTP या UPI PIN नहीं माँगता। पैसे चले जाएँ तो तुरंत 1930 पर कॉल करें।

### Family (Kannada)

> ಎಲ್ಲರಿಗೂ ನಮಸ್ಕಾರ! ಸಂದೇಶ, ಲಿಂಕ್ ಅಥವಾ QR ಕೋಡ್ ಮೋಸವೇ ಎಂದು ಕ್ಲಿಕ್ ಅಥವಾ ಪಾವತಿ ಮಾಡುವ ಮೊದಲು ಪರಿಶೀಲಿಸಲು Satark ಎಂಬ ಉಚಿತ ಸಾಧನ ಮಾಡಿದ್ದೇನೆ. Telegram ನಲ್ಲಿ ಸಂದೇಶವನ್ನು ಇದಕ್ಕೆ ಫಾರ್ವರ್ಡ್ ಮಾಡಿ: <bot link>
> ಇದು ನಿಮ್ಮ ಸಂದೇಶಗಳನ್ನು ಇಟ್ಟುಕೊಳ್ಳುವುದಿಲ್ಲ. ಇದು ತಪ್ಪಾಗಬಹುದು, ಸಂದೇಹವಿದ್ದರೆ ನನಗೆ ಕರೆ ಮಾಡಿ. ಬ್ಯಾಂಕ್ ಎಂದೂ OTP ಅಥವಾ UPI PIN ಕೇಳುವುದಿಲ್ಲ. ಹಣ ಕಳೆದುಹೋದರೆ ತಕ್ಷಣ 1930 ಗೆ ಕರೆ ಮಾಡಿ.

Have the Hindi and Kannada versions checked by a native speaker in the family before sending.

### College

> Built an open-source scam checker for Indian SMS/WhatsApp messages, links and UPI QR codes: Go + Python on Kubernetes, sandboxed link scanning, PII redacted before any LLM call. Try the bot: <bot link> · PWA: <site> · Code: <repo>. Feedback and bug reports very welcome, especially false positives.

### RWA (for the secretary to forward)

> Dear residents, many of us are getting fake KYC, electricity-bill and courier messages. Satark is a free, non-commercial tool that checks a message, link or QR code before you act on it: <bot link>. It does not store your messages and it is not a government service. If you have lost money, call 1930 immediately or report at cybercrime.gov.in.

## During each wave

- Watch the Grafana launch dashboard twice a day; triage 👎 feedback daily (it's the best false-positive signal).
- Keep a running log in `docs/launch/launch-log.md`: date, wave, issue, fix.
- Reply to every person who reports a problem within a day.
- Don't change scoring rules mid-wave without a quick regression run (`eval/`).

## After wave 3

- Retro in `docs/launch/`: what broke, what people actually used, top 5 requests.
- Decide on public launch (blog post, video, Product Hunt / Hacker News).

This plan is carried out by a person sharing links in real groups.
