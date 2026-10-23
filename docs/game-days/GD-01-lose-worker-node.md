# GD-01: Lose a worker node

| | |
| --- | --- |
| Status | Executed on 2026-10-23 (on-prem lab) |
| Facilitator | guru-bharadwaj20 (powered the VM off and on) |
| Responder(s) | agent (no human responder in the lab run) |
| Script | `tests/chaos/gd-01-lose-worker.sh` (power actions done by hand with `VBoxManage`) |
| Expected duration | 45 minutes |

## Hypothesis

When `worker-2` is powered off without a drain:

- `KubeNodeNotReady` fires within 2 minutes.
- Pods on the node are marked for deletion after the default 300 s
  `node.kubernetes.io/unreachable` toleration and rescheduled on `worker-1`.
- Stateless services with 2+ replicas and a PDB see no user-facing errors
  beyond brief retries; single-replica services are unavailable for up to
  ~6 minutes.
- If the CNPG primary was on `worker-2`, CNPG promotes the replica within
  about 60 s (see GD-02).

## Blast radius and abort criteria

- Scope: one of two workers; on-prem cluster only.
- Abort if the public probe fails continuously for more than 10 minutes, or
  `worker-1` shows memory pressure.
- Abort action: `tests/chaos/gd-01-lose-worker.sh revert`.

## Procedure

1. `PROBE_URL=https://api.satark.example/healthz tests/chaos/gd-01-lose-worker.sh observe`
2. `... inject` (starts a 10 minute probe)
3. Responder works from the `KubeNodeNotReady` alert. Expected path:
   `kubectl get nodes`, `kubectl satark doctor`, RB-001 / RB-003.
4. Record when pods were rescheduled: `kubectl get events -A --field-selector reason=TaintManagerEviction`.
5. `... revert` and confirm pods rebalance.

## What to look for

- Did any Deployment have all replicas on the lost node (missing
  topology spread constraints)?
- Did PVC-backed pods get stuck in `ContainerCreating` because local volumes
  cannot move? Which services depend on node-local storage?
- Did Valkey or NATS lose quorum or data?

## Timeline (UTC)

| Time | Event |
| --- | --- |
| 14:17:55 | Baseline. worker-2 held the CNPG primary `satark-pg-1`, `valkey-0`, one of two Envoy proxies and one of two text-analyzer pods |
| 14:23:38 | worker-2 hard power-off (`VBoxManage controlvm satark-worker-2 poweroff`); first probe failures |
| 14:24:23 | worker-2 `NotReady` (45 s) |
| 14:24:29 | CNPG promoted `satark-pg-3` on worker-1 (primary moved within 51 s of the power-off) |
| ~14:28:40 | `TaintManagerEviction` of the four pods (300 s toleration) |
| 14:28:40 | Replacement Envoy proxy and text-analyzer `Pending`: worker-1 had 97% of memory and 94% of CPU requested. `valkey-0` stayed `Terminating` |
| 14:33:54 | worker-2 powered on |
| 14:34–14:49 | worker-2 hung in the initramfs (`Begin: Loading essential drivers ...`), no network, for 15 min |
| 14:49:58 | VM reset by the operator |
| 14:51:03 | worker-2 `Ready` (about 1 min after the reset) |
| 14:51:26 | All pods Running, `satark-pg` 3/3 (primary stays `satark-pg-3`) |

## Results

Probe: one `POST /v1/scan/text` per second through the public gateway
(`gd-01-lose-worker-20261023-probe.txt`).

| Measure | Value |
| --- | --- |
| Time to alert | none: the lab runs no Prometheus/Alertmanager. `NotReady` visible in `kubectl get nodes` after 45 s |
| Time to diagnose | about 3 min after eviction: `kubectl describe pod` showed `Insufficient memory/cpu` on worker-1 |
| Pods rescheduled after | ~300 s evicted; Envoy proxy and text-analyzer never scheduled while the node was down; `valkey-0` came back only with the node |
| Probe failures / total | 42 / 1 252 (3.4%) from power-off to node Ready: 24 × `503`, 18 timeouts; baseline before the power-off was 5 / 290 (1.7%) |
| Latency during the outage | p50 111 ms, p95 2.0 s, p99 3.0 s |
| Hypothesis held? | Partly. NotReady, CNPG failover and the 300 s eviction were as expected and the API kept answering. Rescheduling failed: one worker cannot hold the other's pods |

## Findings

- **No N+1 capacity.** With worker-2 gone, worker-1 already had 97% of
  its memory and 94% of its CPU requested, so the second Envoy proxy and the
  second text-analyzer stayed `Pending` for 22 minutes. The service ran on
  one replica of each. The lab VMs are sized for two workers sharing the
  load, not for one carrying it.
- **Postgres failover worked.** CNPG promoted a replica on worker-1 within
  about 51 s of the power-off, with no manual step, and the old primary
  rejoined as a replica.
- **Valkey is a single pod on one node.** `valkey-0` is a StatefulSet
  replica, so Kubernetes will not start it elsewhere while the node is
  unreachable (it cannot rule out two writers). The cache was gone for the
  whole outage; the API kept answering because the cache is optional, which
  is the intended degradation.
- **The node did not come back by itself.** After the hard power-off,
  worker-2 hung in the initramfs for 15 minutes with no network and needed
  a reset. In production that is a node replacement, not a wait.
- **Short error bursts, not an outage.** Failures came in bursts around the
  power-off and the eviction (Envoy still sending to endpoints on the dead
  node, as in the upgrade test). The 1.7% baseline failure rate before the
  injection is lab noise on the same host and means the true cost of the
  node loss is about 1.7 points.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| No N+1 capacity | Size each worker to carry the other's requests (lab: raise worker memory, or lower requests on platform pods); add a capacity check to SOP-01 and a `kube_node` requests alert | guru-bharadwaj20 | |
| Envoy sends to endpoints on a dead node | Passive health checks and retries on the gateway (same action as the upgrade-under-load report) | guru-bharadwaj20 | |
| Valkey single replica | Accept for a cache; document in RB-003 that `valkey-0` waits for the node, and how to force-delete it if the node is gone for good | guru-bharadwaj20 | |
| Node hung on boot after power loss | RB-002: if a node is not back 5 min after power-on, reset it from the console; in the cloud, let the ASG replace it | guru-bharadwaj20 | |
| No alerting in the lab | Run the alert path on the AWS cluster in GD-08 | guru-bharadwaj20 | |

Evidence: `docs/game-days/logs/gd-01-lose-worker-20261023*`.
