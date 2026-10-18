# GD-07: Disk pressure on a node

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab) |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
| Script | `tests/chaos/gd-07-disk-pressure.sh` |
| Expected duration | 40 minutes |

## Hypothesis

When free space on `worker-2`'s container filesystem drops below 10%:

- `NodeFilesystemAlmostOutOfSpace` (node-exporter) fires first, at 15% free.
- The kubelet sets `DiskPressure=True` and taints the node
  `node.kubernetes.io/disk-pressure:NoSchedule`.
- The kubelet first garbage-collects unused images, then evicts pods,
  BestEffort first, then Burstable pods exceeding their ephemeral-storage
  requests. Guaranteed pods are evicted last.
- Evicted pods are recreated elsewhere; evicted objects remain as `Failed`
  with reason `Evicted` until cleaned up.
- `sandbox-browser` (large image, writes screenshots to emptyDir) is the most
  likely to be evicted.

## Blast radius and abort criteria

- Scope: one worker. Abort if free space drops below 3% (risk to etcd or
  the kubelet itself) or the CNPG instance on the node is evicted.
- Abort action: `... revert`.

## Procedure

1. `... observe`, then `... inject`.
2. Responder starts from the filesystem alert. Expected path: `kubectl
   describe node` (DiskPressure, taint), `kubectl get pods -A | grep Evicted`,
   SSH and `du -xh --max-depth=2 /var/lib | sort -h`, find the filler.
3. `... revert`.

Abort watch: CNPG deletes an evicted instance pod immediately, so look for
events, not `Failed` pods:
`kubectl get events -A --field-selector reason=Evicted -w | grep satark-pg`.

## Things to check

- Do all pods set `ephemeral-storage` requests and limits?
- Is container log rotation (`containerLogMaxSize`) configured?
- Did image GC free enough to matter? How big are our images?

## Results

Lab: `NODE=worker-1 TARGET_FREE_PCT=7`. worker-1 was chosen over the
default worker-2 because worker-2 held the CNPG primary; worker-1 held the
two replicas. The root filesystem is 31 GB and holds containerd, the logs
and the local-path PVs (Postgres data). There is no `sandbox-browser` in
the lab. The probe sent `POST /v1/scan/text` once a second.

| Measure | Value |
| --- | --- |
| Time to alert | n/a (no Prometheus / node-exporter alerts in the lab) |
| Time to DiskPressure | 3 s (filler at 15:58:26, condition 15:58:29, `disk-pressure:NoSchedule` taint 15:58:30) |
| Pods evicted | `satark-pg-3`, `satark-pg-4` (CNPG replicas, Burstable), `gateway-api` (Burstable), `metallb-speaker` (DaemonSet), all within ~15 s; then the speaker 8 more times |
| Not evicted | `cilium`, `cilium-envoy` (critical pods; the kubelet logged `cannot evict a critical pod`) |
| Image GC | freed ~6.8 GB (7% → 29% free) about 2 min in, after the evictions had made images unused |
| Time to diagnose | under 1 min: `kubectl describe node` (DiskPressure, taint) and `Evicted` events naming ephemeral-storage |
| Probe failures | 0 / 300 |
| DiskPressure cleared | 102 s after the filler was removed (5 min after the last pressure observation) |

Raw logs: `logs/gd-07-disk-pressure-20261018.log` and the 15 s watch
`logs/gd-07-disk-pressure-watch-20261018.log`.

Caveat: the Windows host was running heavy WSL jobs alongside the lab VMs.

## Timeline (UTC)

| Time | Event |
| --- | --- |
| 15:58:26 | 26 GB filler written; 2.2 GB (7%) free |
| 15:58:29 | `DiskPressure=True`, taint added |
| ~15:58:40 | `metallb-speaker`, `satark-pg-3`, `satark-pg-4`, `gateway-api` evicted ("low on resource: ephemeral-storage") |
| 15:58:40 → 16:04 | The `metallb-speaker` DaemonSet pod is recreated and evicted again 8 times |
| ~16:00:44 | Image GC: 29% free; kubelet `able to reduce resource pressure without evicting pods` |
| 16:04:09 | Revert: filler removed |
| 16:05:53 | DiskPressure cleared |
| 16:06:34 | CNPG back to 3/3 |
| after | Rescheduled `gateway-api` pod in `ErrImagePull` on worker-1: GC had deleted the side-loaded image (lab-only; fixed by copying the image back from worker-2) |

## What we learned

- **The abort criterion was hit and the watchdog missed it.** The doc says
  abort if a CNPG instance on the node is evicted. Both replicas were
  evicted in the first 15 s. The run's watchdog looked for `Failed` pods,
  but the CNPG operator deletes an evicted instance pod straight away, so
  there was never a `Failed` pod to see. The run continued for 5 minutes
  with the primary alone; CNPG relaxed `minSyncReplicas` to keep writes
  going ("Ignore minSyncReplicas to enforce self-healing", as in GD-06).
  Watch `Evicted` *events* instead (`kubectl get events -A
  --field-selector reason=Evicted`). The procedure now says so.
- **Eviction order was not what the hypothesis said.** The kubelet ranks
  first by "usage above request". No satark pod (and no CNPG pod) sets an
  `ephemeral-storage` request, so every pod counts as over its request.
  Postgres replicas using 76 KiB of ephemeral storage were among the first
  to go. Image GC freed the real space only *after* the evictions, once
  the evicted pods' images were unused.
- The Postgres data lives on local-path PVs on the same filesystem, so
  filling the disk also threatens the database files directly. That is why
  the abort criterion matters.
- A DaemonSet that does not tolerate the condition loops. `metallb-speaker`
  was evicted 9 times in 5 minutes; each new pod tolerated the
  `NoSchedule` taint (DaemonSet default) and was evicted again. L2
  announcement moved to the other speakers, so the VIP stayed up (0
  failed probes).
- Image GC can delete images that cannot be pulled back. In the lab, that
  is any side-loaded image. In production it is a reminder that a pull
  outage plus disk pressure turns into pods that cannot start.
- Log rotation: kubelet defaults (`containerLogMaxSize` 10 Mi × 5 files)
  are in effect; logs were not the problem here.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| Watchdog missed CNPG evictions | Procedure: watch `Evicted` events, not `Failed` pods (below) | guru-bharadwaj20 | ✅ |
| CNPG pods evicted first for using 76 KiB | Give the CNPG cluster a `priorityClassName` above the apps, and `ephemeral-storage` requests to every chart via `satark-common` | guru-bharadwaj20 | |
| Postgres PVs share the root filesystem | Separate disk/LV for local-path PVs in the lab Vagrantfile; on cloud, EBS volumes already are | guru-bharadwaj20 | |
| Alert timing (`NodeFilesystemAlmostOutOfSpace`) untested | Re-run on the cloud cluster with node-exporter | guru-bharadwaj20 | |
