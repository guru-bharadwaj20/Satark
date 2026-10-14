# Product KPIs

Three numbers tell us whether Satark is useful. They're reviewed every Monday and logged in `docs/launch/kpi-log.md`.

| KPI | Definition | Target after launch wave 3 |
|---|---|---|
| **Weekly active users (WAU)** | Distinct users who completed at least one check in the last 7 days, across bot, PWA and API | 300 |
| **Scans** | Completed checks (text, link, QR, lookup) in the last 7 days | 1,000 |
| **Helpful-rating rate** | 👍 presses ÷ all 👍/👎 presses on results, last 7 days | ≥ 75% |

Supporting signals: scans per active user (engagement), share of checks rated (aim > 10% so the helpful rate means something), 👎 on `high` results (false-positive proxy).

## Metric contract

Each channel emits these; names follow the `satark_` prefix convention.

| Metric | Type | Labels | Emitted by |
|---|---|---|---|
| `satark_checks_total` | counter | `channel` (telegram, pwa, api), `kind` (text, url, qr, lookup) | gateway-api, per completed check |
| `satark_feedback_total` | counter | `rating` (helpful, not_helpful), `risk` | telegram-bot and PWA, per button press |
| `satark_active_users` | gauge | `window` (1d, 7d, 30d) | gateway-api: `PFCOUNT` over daily HyperLogLog keys in Valkey |

### Counting active users without storing who they are

The gateway `PFADD`s an HMAC of the caller (Telegram user ID, PWA install ID, or API key) into `satark:wau:<YYYY-MM-DD>` (expiry 31 days) and publishes `PFCOUNT` over the last 1/7/30 keys as `satark_active_users`. HyperLogLog only estimates cardinality (±1%) and cannot list members, so this never becomes a user list. Deleting a user via `/forget` doesn't need to touch it.

## Weekly report

```bash
kubectl -n monitoring port-forward svc/prometheus-operated 9090 &
PROM_URL=http://localhost:9090 scripts/kpi-report.sh | tail -1 >> docs/launch/kpi-log.md
```

`--json` prints the same numbers for dashboards or a bot message.

## Grafana

Add these panels to the launch dashboard:

- WAU: `max(satark_active_users{window="7d"})`
- Scans per day: `sum(increase(satark_checks_total[1d]))` by `channel`
- Helpful rate: `sum(increase(satark_feedback_total{rating="helpful"}[7d])) / sum(increase(satark_feedback_total[7d]))`
- False-positive proxy: `sum(increase(satark_feedback_total{rating="not_helpful",risk="high"}[7d]))`

## Verify

```bash
# Each metric exists (expect non-empty results)
for m in satark_checks_total satark_feedback_total satark_active_users; do
  curl -fsS --get "$PROM_URL/api/v1/query" --data-urlencode "query=count($m)" | jq -r ".data.result | length"
done
```
