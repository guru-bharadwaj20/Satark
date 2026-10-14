# Load test: first bottleneck

Status: **procedure ready; results pending a run on the live cluster.**

## Method

1. `tests/load/run.sh smoke` to confirm the endpoints and thresholds work.
2. `tests/load/run.sh load` for a 15 minute baseline at up to 50 req/s.
3. `tests/load/run.sh stress` and note the arrival rate at which the first
   threshold fails.
4. During the stress run, watch in Grafana:
   - per-service p95 latency and `satark_*_errors_total`
   - CPU throttling: `rate(container_cpu_cfs_throttled_periods_total[1m])`
   - Postgres: active connections vs `max_connections`, lock waits
   - Valkey: `connected_clients`, command latency
   - NATS: pending messages per consumer
   - HPA/KEDA replica counts (`autoscale-*.txt` snapshots)
5. Identify the first component to saturate, fix it, re-run `stress` and
   compare the breaking rate.

## Likely candidates to check first

| Suspect | Symptom | Typical fix |
| --- | --- | --- |
| `text-analyzer` CPU limit | High throttling, p95 climbs linearly | Raise CPU limit, add uvicorn workers, HPA on CPU |
| Postgres connections | `too many clients`, gateway 5xx | PgBouncer pooler (CNPG `Pooler`), smaller app pools |
| `url-intel` outbound DNS/RDAP | `check_url` p95 > 3 s | Cache RDAP/WHOIS in Valkey, lower timeouts |
| `gateway-api` rate limiter | 429s at modest rate | Per-key limits instead of global |

## Results

| Run | Date | Breaking rate (req/s) | First failing threshold | Saturated component |
| --- | --- | --- | --- | --- |
| Baseline | | | | |
| After fix | | | | |

## Fix applied

- Change:
- Commit:
- Improvement:
