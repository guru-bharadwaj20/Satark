# Service level objectives

Satark is a free public-interest service run on a small cluster, so the
objectives are deliberately modest and measured where users feel them: at
`gateway-api`, the single entry point for the PWA, Telegram bot and API.

| SLO | SLI | Objective | Window | Error budget |
|-----|-----|-----------|--------|--------------|
| `availability` | Share of `gateway-api` requests that do not return 5xx | 99.5 % | 30 days rolling | 0.5 % (~3 h 36 m of full outage) |
| `latency` | Share of `gateway-api` requests served in under 800 ms | 95 % | 30 days rolling | 5 % of requests may be slower |

Excluded from both SLIs: `/healthz`, `/readyz`, `/metrics` and unmatched routes
(`route="unmatched"`, typically scanners probing random paths). 4xx responses
count as good: they are client errors, not service failures.

Why these numbers: a scan answers "should I trust this message right now?",
so a failed request is worse than a slow one, while LLM-backed text analysis
legitimately takes several hundred milliseconds. 95 % under 800 ms leaves room
for the LLM path while still catching regressions.

## Recording rules

Defined in `gitops/platform/monitoring/rules/slo-recording.yaml`, every series
carries `slo` and `service` labels:

| Series | Meaning |
|--------|---------|
| `slo:objective:ratio` | The objective (0.995 / 0.95) |
| `slo:sli_error:ratio_rate{5m,30m,1h,2h,6h,1d,3d,30d}` | Bad-event ratio over each window |
| `slo:error_budget_remaining:ratio` | `1 - (30d error ratio / allowed error ratio)`; 1 = untouched, 0 = spent, negative = overspent |

Burn rate for any window is `slo:sli_error:ratio_rate<w> / (1 - slo:objective:ratio)`.
Prometheus retention is 35 days so the 30-day window is always complete.

## Error budget policy

- **Budget above 50 %:** ship normally.
- **Budget 25–50 %:** releases continue, but every incident gets a written review.
- **Budget below 25 %:** feature work pauses; reliability fixes and rollbacks only.
- **Budget exhausted:** freeze all non-reliability changes until the 30-day
  window recovers above 25 %, except security fixes.
