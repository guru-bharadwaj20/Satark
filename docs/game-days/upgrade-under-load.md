# Upgrade under load: N-1 to N

Status: **executed on 2026-10-18 (on-prem lab), 1.35.9 to 1.36.5.** Task
11.10. Re-run with `tests/load/upgrade-under-load.sh <target-minor> <inventory>`.

## Setup

| Field | Value |
| --- | --- |
| Date (UTC) | 2026-10-18, playbook 16:23:03 to 16:37:59 |
| From version | v1.35.9 |
| To version | v1.36.5 |
| Nodes | 1 control plane (cp-1), 2 workers; VirtualBox VMs on one Windows host |
| Load | k6 `upgrade-soak.js`, constant 20 req/s on `POST /v1/scan/text`, 120 min from 16:21:03 |
| Stack | Cilium 1.18.2 (kube-proxy replacement), Envoy Gateway v1.2.1 (2 proxies), CNPG 1.24.1 (3 instances, `minSyncReplicas: 1`) |
| Playbook | `infra/ansible/playbooks/upgrade.yml` with the new CNPG handling (`roles/k8s_upgrade/tasks/cnpg.yml`) |

The gateway's per-client rate limit was raised for the run (k6 sends from
one IP), with Argo CD auto-sync paused, and put back afterwards.

## Results

From `k6-summary.json` and `k6.csv.gz` (every request, bucketed per
Ansible task with `profile_tasks` timestamps).

| Metric | Value | Target |
| --- | --- | --- |
| Upgrade duration | 14 min 56 s (playbook), all 3 nodes, 0 failed tasks | < 60 min |
| Requests sent, upgrade window (16:23–16:38) | about 17 800 | |
| Failed requests, upgrade window | 2 395 (13.4%) | 0 ideal, < 0.1% ❌ |
| Failed requests after upgrade, 16:38–17:08 | 0 | |
| Failed requests, whole 2 h run | 4 647 / 143 842 (3.2%); 2 252 of them are the host stall at 17:08–17:30 (see caveats) | |
| p95 latency, whole run | 195 ms (p99 4.1 s) | < 800 ms |
| p95 latency in the worst steps | 1.4 s (kubeadm apply), 6.2 s (worker-2 drain) | < 800 ms ❌ |

The upgrade itself works: every node was upgraded in order, nothing
needed a manual step, and Postgres kept 3 instances. Availability during
it does not meet the target.

## Timeline

| Time | Event | Requests / failures in window |
| --- | --- | --- |
| 16:21:03 | k6 starts; baseline | 2 400 / 0 (p95 87 ms) |
| 16:23:07 | Pre-flight; local etcd snapshot (no `etcd-backup` unit on-prem) | 0 failures |
| 16:24:09 | `kubeadm upgrade apply` on cp-1 (249 s); API server restarted, unreachable about 16:25:44–16:29 | 4 976 / 1 040 × `503` (p95 1.4 s) |
| 16:28:18 | cp-1 drain, kubelet upgrade and restart, wait for Ready | 1 700 / 617 × `503` |
| 16:29:44 | worker-1: CNPG had no primary there; maintenance window on | 600 / 40 |
| 16:30:15 | worker-1 drain (40 s), kubelet upgrade, uncordon | 796 / 222 × `503` |
| 16:31:36 | CNPG maintenance off, wait for 3/3 ready (21 s) | 0 |
| 16:32:33 | worker-2: CNPG switchover `satark-pg-1` → `satark-pg-3` (7 s) | 0 |
| 16:32:40 | worker-2 drain (174 s) | 3 422 / 335 (207 × `503`, 128 timeouts; p95 6.2 s) |
| 16:36:00 | worker-2 kubelet upgraded, uncordoned; CNPG back to normal | 160 / 134 timeouts |
| 16:36:37 | CIS hardening re-applied (`cis-hardening.yml`): kubelets and API server restart again | within the line above |
| 16:37:59 | Playbook done | |
| 16:38–17:08 | Steady state on 1.36.5 | 0 failures |
| 17:08–17:30 | Host CPU at 100%: worker-2 VM soft lockups, NotReady 17:21:37–17:25:48 | 2 252 (not caused by the upgrade) |

Failures per minute during the upgrade window: 16:25 18, 16:26 59, 16:27
60, 16:28 55, 16:29 50, 16:30 19, then 6–11 a minute in the worker-2 drain.
That is: the drains cost little, and the control-plane step cost most.

## Findings

- **Most errors came from Envoy routing to a deleted pod while the API
  server was down (1 040 + 617 × `503`).** The Envoy Gateway controller
  lost its leader lease when the API server restarted and exited three
  times (last at 16:29:42). From about 16:25 the proxies kept sending a
  third of traffic to `10.244.2.77:8080`, a gateway pod that had been
  deleted at 16:20, and logged `503 UF upstream_reset_before_response_started{connection_timeout}`
  (1 701 of the 1 923 `UF` lines in the two proxies' access logs name that
  address).
  It lasted until the controller resynced at about 16:30. The data plane
  should survive a control-plane restart; here it served a stale endpoint
  list with no retry.
- **Evicted pods still got traffic.** In the worker-1 drain, pod `5f6fj`
  got 219 × `503 UF` after it was evicted, until Envoy's endpoints caught
  up. The gateway has no `preStop` delay, so it stops before the endpoint
  removal reaches Envoy.
- **The new gateway pod crash-looped on startup.** The replacement
  `fxg5z` exited twice with `store: ping: context deadline exceeded`.
  CoreDNS had been moved at 16:28:21, and an earlier log line shows
  `lookup valkey... i/o timeout`. A single failed DB/DNS check at startup is
  fatal instead of retried.
- **The worker-2 drain took 174 s and was the slowest step for users (p95
  6.2 s).** worker-2 held nearly every platform pod (Argo CD, cert-manager,
  Envoy Gateway, both Envoy proxies, CoreDNS, Valkey, the CNPG primary), so
  everything moved to worker-1 at once and pulled images there. The Envoy
  PDB (`minAvailable: 1`) held the second proxy until its replacement was
  Ready (about 108 s), and that proxy's `preStop` hook then hit its 60 s
  timeout (`FailedPreStopHook`). The 128 timeouts are from those minutes.
- **CNPG handling works.** The new `cnpg.yml` turned on the maintenance
  window before each worker drain, switched the primary off worker-2 in 7 s,
  and waited for 3/3 ready afterwards. The `satark-pg-primary` PDB never
  blocked a drain. Placement after the run (primary `-3` and replica `-4`
  on worker-1, `-1` on worker-2) is the safe layout from GD-06.
- **The playbook restarts the control plane twice.** `upgrade.yml` re-applies
  CIS hardening at the end (package upgrades reset file modes), which
  restarts the kubelets and the API server again at 16:36:37.
- **etcd snapshot.** The on-prem cluster has no `etcd-backup` unit, so the
  playbook now takes a local `etcdctl snapshot` through `kubectl exec`.
  p95 was 1.7 s while it ran (6 s), with no failures.
- **Version skew.** Cilium 1.18 does not list 1.36 among tested Kubernetes
  versions. Nothing failed, but upgrade Cilium first next time.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| Upgrade path for CNPG (primary PDB blocks drains) | `roles/k8s_upgrade/tasks/cnpg.yml`: maintenance window + switchover before drain, wait for ready after; SOP-01 updated | guru-bharadwaj20 | ✅ |
| No etcd backup unit on-prem | Playbook takes a local snapshot when `etcd-backup.service` is missing; install `roles/etcd_backup` on-prem | guru-bharadwaj20 | partly |
| Inventory still said 1.35 | `kubernetes_minor: "1.36"` in `inventory/onprem/group_vars/k8s_cluster.yml` (SOP-01 step 6) | guru-bharadwaj20 | ✅ |
| Envoy serves a stale endpoint while the API server is down | Upgrade Envoy Gateway (v1.2.1 is old) and add a `BackendTrafficPolicy` with retries on `connect-failure`/`reset` and passive health checks (outlier detection) | guru-bharadwaj20 | |
| Evicted gateway pods still get traffic | `preStop: sleep 10` and `terminationGracePeriodSeconds: 30` on gateway-api, text-analyzer, qr-decoder | guru-bharadwaj20 | |
| Gateway exits on one failed DB ping at startup | Retry with backoff for 60 s before exiting; readiness, not startup, should gate on the DB | guru-bharadwaj20 | |
| Single control plane: API restart = data-plane drift | 3 control-plane nodes in the cloud cluster (task 5.x); on-prem accept it | guru-bharadwaj20 | |
| Platform pods all on one worker | Spread constraints for Envoy proxies and CoreDNS; Envoy proxy `preStop` drain shorter than its 60 s timeout | guru-bharadwaj20 | |
| Cilium version skew | Upgrade Cilium before the next Kubernetes minor (SOP-01 pre-check) | guru-bharadwaj20 | |
| k6 cannot be stopped from Git Bash | Documented in the script: set `DURATION` to upgrade time + 10 min in the lab | guru-bharadwaj20 | ✅ |
| `kubectl set env` survives Argo re-sync | After a soak, remove the env with `set env NAME-`; Argo does not prune extra list entries | guru-bharadwaj20 | ✅ |

## Caveats

- Single-host lab. The host also ran WSL GPU jobs and was at 100% CPU for
  parts of the run. From 17:08 the worker-2 VM had soft lockups
  (`watchdog: BUG: soft lockup`, `systemd-networkd` watchdog timeout) and
  was NotReady from 17:21:37 to 17:25:48; it recovered by itself before the
  300 s eviction. Those 2 252 failures are excluded from the upgrade
  numbers above.
- k6 ran from the same host, so some latency outliers are the client.
- `summary.txt` was written by hand from `k6-summary.json`: the script
  file was edited while it was running, and bash failed on its last lines
  after `nodes-after.txt`. The upgrade and the k6 data were not affected.
- Evidence: `docs/game-days/logs/upgrade-under-load-20261018-*`.
