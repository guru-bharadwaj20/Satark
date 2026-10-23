# Load test: first bottleneck

Status: **done on the lab cluster, 2026-10-17 and 2026-10-18.**

Lab: kubeadm 1.35 on three VirtualBox VMs (2 vCPU, 4 GB each; two workers),
Envoy Gateway on a MetalLB IP, k6 on the same Windows host. Run with
`tests/load/run.sh stress -c tests/load/lab.json --out csv=...` and the
gateway-api rate limit raised for the test, so the limiter was not what
failed.

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
| Baseline | 2026-10-17 | ~95 | text `p(95)<500` at 99 req/s (697 ms), then every threshold | `worker-2`: 96% CPU, soft lockups, node `NotReady` |
| After fix | 2026-10-18 | ≥160 | text `p(95)<500` at 160 req/s (499 ms), no errors | none in the cluster; the host ran out of CPU (see below) |

Per 30 s window (from the k6 CSV):

| Arrival rate | Baseline text p95 / errors | After fix text p95 / errors |
| --- | --- | --- |
| 50 req/s | 239 ms / 0% | 15 ms / 0% |
| 90 req/s | 342 ms / 0% | 27 ms / 0% |
| 100 req/s | 697 ms / 0%, collapse next window | 25 ms / 0% |
| 110-130 req/s | 94% failed | 89 ms / 0% |
| 160 req/s | down | 499 ms / 0% |

### What failed in the baseline

All pods had piled onto `worker-2`: both Envoy proxies, both replicas of
`text-analyzer` and `qr-decoder`, `gateway-api`, Valkey, Postgres, Argo CD
and CoreDNS. Three things caused it:

1. **CPU requests far below real use.** `gateway-api` asked for 25m and used
   about 430m at 90 req/s, so the scheduler saw a nearly empty node.
2. **A soft spread constraint that rollouts defeat.** The chart's
   `ScheduleAnyway` hostname spread counted the old ReplicaSet's pods, so a
   rollout put every new pod on whichever node the old ones were leaving.
   The same happened to the Envoy proxies under a hard constraint.
3. **No rebalancing.** Pods evicted by an earlier drain of `worker-1` never
   moved back.

At about 100 req/s `worker-2` reached 96% CPU, the guest kernel logged soft
lockups, the kubelet stopped reporting and the node went `NotReady`. Two
replicas of everything did not help, because both were on that one node.

### What limited the re-run

After the fix both workers carried an even share (75% and 82% CPU at
150 req/s) and the HPA took `gateway-api` from 2 to 4 replicas. Latency was
still under the threshold at 160 req/s. At that point other work on the same
Windows host (a WSL VM using 28 of 32 logical CPUs) started, the VMs' vCPUs
were starved (`rcu_preempt kthread starved` on the `worker-1` console) and
`worker-1` went `NotReady`. 160 req/s is therefore a lower bound, set by the
host, not by Satark. For a clean ceiling, run k6 from another machine and
keep the host otherwise idle; on this lab, 2 workers with 2 vCPU each are
nearly full at 160 req/s, so the next limit is node CPU, fixed by adding a
worker (SOP-02) rather than by tuning.

## Fix applied

- Change:
  - `gateway-api` dev values: CPU request 200m (from 25m), HPA 2-4 replicas
    at 70% CPU.
  - Envoy proxies: hard hostname spread (`DoNotSchedule`,
    `nodeTaintsPolicy: Honor` so a drain can still move a replica).
  - `satark-common` chart and the Envoy proxies: `matchLabelKeys:
    [pod-template-hash]`, so only the new ReplicaSet counts during a
    rollout.
- Commit: `bd30e96` (requests, HPA, Envoy spread), `0f6f679`
  (`matchLabelKeys`).
- Improvement: breaking rate from about 95 req/s to at least 160 req/s
  (+70%), text p95 at 100 req/s from 697 ms to 25 ms, and losing one node no
  longer takes every replica with it.
- Found after the re-run: both `envoy-gateway` controller replicas had been
  rescheduled onto `worker-1`. When it locked up, the proxies kept two dead
  `gateway-api` pod IPs and half the requests failed with `503 No route to
  host` until the controller was restarted. Fixed by a hard spread for the
  controller (`values.yaml`) and a Gateway-wide `BackendTrafficPolicy`
  (passive health checks plus retry on connect failure).
- Follow-ups:
  - Set real CPU requests on `text-analyzer` and `qr-decoder` from the
    `kubectl top` snapshots in the run directory.
  - Add the descheduler (`RemovePodsViolatingTopologySpreadConstraint`) or
    restart Deployments after a drain, so pods spread again.
  - Re-run `stress` with k6 off the host to find the real ceiling.
