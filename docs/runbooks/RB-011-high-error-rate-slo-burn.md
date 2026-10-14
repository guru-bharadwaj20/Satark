# RB-011: High API error rate or fast SLO burn

| | |
|---|---|
| **Alerts** | `SatarkAvailabilityBudgetFastBurn`, `SatarkLatencyBudgetFastBurn` (critical); `SatarkAvailabilityBudgetSlowBurn`, `SatarkLatencyBudgetSlowBurn`, `SatarkHighErrorRate`, `SatarkHighLatency`, `SatarkLLMErrorsHigh` |
| **Severity** | Fast burn: critical, act now. Slow burn: warning, fix within the working day. |
| **Impact** | People checking a suspicious message get errors or slow answers. A fast burn at 14.4x spends 2 % of the 30-day error budget per hour. |
| **Dashboards** | Satark / SLOs and error budget, Satark / Scan pipeline |
| **SLO definitions** | [docs/observability/slos.md](../observability/slos.md) |

## 1. Triage (5 minutes)

1. Open **Satark / SLOs and error budget**: which SLO (availability or latency),
   which window, how much budget is left?
2. Open **Satark / Scan pipeline** and set `service` to All:
   - Is the error ratio / latency high in `gateway-api` only, or also in a
     downstream service (`url-intel`, `text-analyzer`, `qr-decoder`, `reputation`)?
     The downstream service with the earliest spike is usually the cause.
   - Which route? (`Latency percentiles by route`, `Requests by status code`)
   - LLM row: are LLM errors or latency up?
   - Caches row: did the hit ratio collapse (Valkey down → every request is a miss)?
3. Check what changed:

```bash
kubectl -n argocd get applications -o wide          # recent syncs, degraded apps
kubectl -n satark get pods -o wide                  # restarts, pending pods
kubectl -n satark rollout history deploy/gateway-api
```

4. Read the errors (Grafana Explore, Loki):

```logql
{namespace="satark", level="ERROR"} | json | line_format "{{.service}} {{.msg}} {{.error}}"
sum by (service, msg) (count_over_time({namespace="satark", level="ERROR"} | json [5m]))
```

Use `trace_id` from a failing line to follow one request across services:
`{namespace="satark"} | trace_id="<id>"`.

## 2. Common causes and mitigations

| Signal | Likely cause | Mitigation |
|---|---|---|
| Spike starts at a deploy | Bad release | Revert the commit in Git (Argo CD syncs it). See RB-001 section 4. |
| 5xx from one downstream, pods restarting | Crash loop / OOM | RB-001 |
| `dial tcp ... i/o timeout` / `no such host` in logs | DNS or network | RB-005; check Hubble drops |
| Postgres errors (`too many connections`, timeouts) | DB saturated or failing over | `kubectl -n satark-data get cluster satark-pg`; reduce pool sizes; wait for failover to finish |
| Cache hit ratio dropped to 0 | Valkey down | `kubectl -n satark-data get pods -l app.kubernetes.io/name=valkey`; services must degrade to direct lookups, check they do |
| LLM errors/timeouts up | LLM provider outage or rate limit | Text analysis falls back to rule-based scoring. Confirm the fallback works; if errors still reach users, switch the LLM feature flag off via Git. |
| Latency SLO only, error ratio normal | Slow LLM or external lookups (RDAP/WHOIS, Safe Browsing) | Check per-route latency; tighten client timeouts so the request returns a partial verdict instead of hanging |
| Traffic spike, CPU throttling | Load (viral forward, bot) | Scale: `kubectl -n satark scale deploy/<svc> --replicas=N` then make it permanent in Git; check rate limits at the gateway |

Prefer mitigation that restores service (rollback, scale, feature off) over a
root-cause fix during a fast burn.

## 3. Communicate

- Fast burn lasting more than 15 minutes: declare an incident in the ops
  channel, name an incident lead, post updates every 30 minutes.
- If users are visibly affected, post a short status message on the public
  Telegram channel. Do not promise a time to fix.

## 4. Verify

- Burn rate (1h) on the SLO dashboard is below 1 and falling.
- Error ratio and p95 latency per service back to baseline for 30 minutes.
- Critical alert resolved in the Telegram ops channel (the 5m/30m short
  windows make fast-burn alerts resolve quickly once fixed).

## 5. Follow-up

- Record budget consumed by the incident (SLO dashboard, *Error budget remaining*).
- Apply the error budget policy in `docs/observability/slos.md`: below 25 %
  remaining means feature work pauses.
- Write an incident review for any fast burn, with action items as issues.
