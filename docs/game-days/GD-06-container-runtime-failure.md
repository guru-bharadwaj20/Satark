# GD-06: Container runtime failure

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab), two variants |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
| Script | `tests/chaos/gd-06-runtime-failure.sh` |
| Runbook under test | RB-002 (node NotReady) |
| Expected duration | 40 minutes |

## Hypothesis

With containerd stopped and masked on `worker-1`:

- Existing containers keep serving for a while (shims are independent of
  the daemon), so user impact is delayed and partial.
- Within ~3 minutes the kubelet reports `PLEG is not healthy` /
  `container runtime is down` and the node becomes `NotReady`.
- After the 300 s unreachable toleration, pods are evicted and recreated on
  other nodes; old containers on `worker-1` keep running as orphans until
  containerd returns and the kubelet kills them.
- Liveness probes on `worker-1` fail to restart anything (no runtime), so
  any container that crashes in this window stays dead.

The interesting part is the difference from GD-01: the node is reachable by
SSH and still answers some traffic, which can mislead the responder.

## Blast radius and abort criteria

- Scope: one worker. Abort if the other worker shows pressure or the probe
  fails for more than 10 minutes.
- Abort action: `... revert`.

## Procedure

1. `... observe`, then `... inject`.
2. Responder starts from `KubeNodeNotReady`. Expected path: `kubectl describe
   node` (Ready condition message), SSH to the node, `systemctl status
   containerd`, `journalctl -u kubelet`, `crictl ps` fails.
3. `... revert` and confirm orphaned containers are cleaned up
   (`crictl ps` matches `kubectl get pods --field-selector spec.nodeName=worker-1`).

`MODE=daemon` (the default) stops containerd and then masks it, which is
the failure the hypothesis describes. `MODE=everything` is the original
`systemctl mask --now`, which turned out to kill every container on the
node as well (see below).

## Results

Lab: 3 VMs (cp-1, worker-1, worker-2), Kubernetes 1.35.9, containerd
2.2.1. CNPG `satark-pg` has 3 instances and `minSyncReplicas: 1`. Its
primary `satark-pg-1` was on worker-2 and both replicas (`-3`, `-4`) were
on worker-1, the node under test. Two loads ran during each variant:

- a probe on `POST /v1/scan/text` once a second, for 600 s;
- a writer inserting a row through `satark-pg-rw` every 0.5 s, from the
  primary's pod, logging each acknowledged row number.

The first run used the script as written (`mask --now`). Because it killed
everything, the daemon-only variant the hypothesis describes was run 20
minutes later.

| Measure | Run 1: `mask --now` (everything dies) | Run 2: stop, then mask (daemon only) |
| --- | --- | --- |
| Inject | 15:45:03 | 16:06:57 |
| Time to NotReady (taint added) | 30 s (15:45:33) | 35 s (16:07:32) |
| Ready message | `container runtime is down`; `PLEG is not healthy` added after 3 min | same |
| containerd-shim processes on the node | 0 | 5 (unchanged for 12 min) |
| Pods evicted (NoExecute + 300 s) | 15:50:36 | about 16:12:36 |
| Probe failures | 54 / 600 (10 × `503`, 44 × timeout) | 0 / 600 |
| Postgres writes | 733 acknowledged, 0 errors, one INSERT hung 136 s | 717 acknowledged, 0 errors, no gap over 3 s |
| Time to alert | not measured (no Prometheus in the lab); `SatarkNodeNotReady` has `for: 3m`, so about 3.5 min after inject | same |
| Time to diagnose | immediate: the node's Ready message names the runtime | same |
| Revert to node Ready | 7 s | 6 s |
| Revert to Postgres 3/3 ready | 53 s | 33 s |
| Orphans after revert | none (nothing survived) | none: the kubelet removed the evicted pods and CNPG recreated both replicas |

Run 1 timeline (UTC):

| Time | Event |
| --- | --- |
| 15:45:03 | `systemctl mask --now containerd` on worker-1 |
| 15:45:04–15:45:17 | 9 × `503`: the gateway pod and an Envoy proxy pod on worker-1 died at once; traffic moved to worker-2 |
| 15:45:33 | Node NotReady (NoSchedule taint, NoExecute at 15:45:36) |
| 15:45–15:50 | Pods on worker-1 still show `1/1 Running` and CNPG reports `ready=3`, though no container process is left |
| 15:48:26, 15:48:40 | Primary logs both standbys disconnecting |
| 15:48:38 | Writes stop: the next INSERT waits for a synchronous standby that no longer exists |
| 15:48:42–15:50:54 | 44 probe timeouts: `SaveScan` runs in the request path and waits on the same commit |
| 15:50:36 | NoExecute eviction: pods on worker-1 marked Terminating |
| 15:50:55 | CNPG: "Ignore minSyncReplicas to enforce self-healing". The hung INSERT returns (logged at 135 482 ms), writes resume |
| 15:57:31 | Probe done; revert. Node Ready 7 s later, Postgres 3/3 at 15:58:24 |

Run 2: the node went NotReady just as fast, but the 5 shims and their
containers kept running. The Postgres replicas kept streaming, so
synchronous commits went on with no gap. They were marked Terminating
after the eviction, and `readyInstances` dropped to 1, while their
processes still ran, because the kubelet could not stop them without
containerd. On revert the kubelet stopped the evicted pods, and CNPG
recreated `-3` and `-4` on their local PVCs within 33 s. The node had no
serving gateway pod in this run (see caveats), so a probe result of 0
failures says nothing about worker-1's request path. It only shows that
the database tier survived.

Caveats:
- This is a single-host lab, and the host was also running WSL GPU jobs.
- In run 1, at about 15:48, I cleaned up a leftover text-analyzer pod
  from GD-09. It overlapped the window, but requests succeeded from
  15:45:18 to 15:48:30, so a live text-analyzer replica was serving. The
  15:48:42+ failures line up with the write hang, not with that.
- In run 2, worker-1's gateway pod was in `ErrImagePull`: GD-07's image GC
  had removed the side-loaded image. The images were copied back from
  worker-2 after the run.

## What we learned

- **`mask --now` is not "containerd crashed".** Masking replaces the unit
  with `/dev/null`, so the stop that follows runs with default settings
  (`KillMode=control-group`) instead of the real unit's `KillMode=process`,
  and it kills every shim. The containers' stdout and stderr pipes were
  gone. Each process died the next time it wrote a log line (probably
  SIGPIPE): the gateway at once, the Postgres standbys about 3 minutes
  later. The script now defaults to stop-then-mask, which leaves the shims
  alone. `MODE=everything` keeps the harsh variant, which is a fair model
  of "the node's runtime cgroup was killed".
- **The node goes NotReady in about 30 s, not 3 minutes.** The kubelet's
  runtime check (`container runtime is down`) trips well before the PLEG
  threshold. The Ready message names the cause directly, so the diagnosis
  is quick once someone reads the node conditions. SSH works and the node
  answers ping, which is the trap the hypothesis predicted.
- **Status goes stale.** For 5 minutes after the containers died, the API
  still showed them `1/1 Running`, and CNPG still counted 3 ready
  instances. The kubelet was alive but could not ask the runtime, so
  nothing corrected the cached status until the NoExecute eviction.
  Anything that trusts pod status (CNPG, endpoints, dashboards) is wrong in
  that window.
- **Synchronous replication plus co-located replicas stops all writes.**
  `minSyncReplicas: 1` needs one live standby. Both standbys were on the
  failed node, so from 15:48:38 every commit waited. Because the gateway
  saves scans in the request path, user requests timed out too. CNPG only
  relaxed the rule at 15:50:55, after the pods were evicted. With 3
  instances on 2 workers, two instances always share a node. Writes
  survive a node loss only if those two are the primary and one replica,
  not both replicas, and the current preferred anti-affinity cannot
  express that.
- **The hypothesis about orphans was wrong in a good way.** Evicted pods
  were not left as orphans after the revert: the kubelet stopped them as
  soon as containerd was back.
- **Script bug:** `observe` sent `bash -c "'$1'"` through ssh. ssh joins
  its arguments into one line, so the quotes in the journalctl filter
  broke (`bash: line 1: container: command not found`, the same class of
  bug as GD-09). `remote()` now sends the script on stdin
  (`bash -s <<<"$1"`). `observe` also prints the shim count, which is the
  quickest way to tell the two variants apart.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| `mask --now` kills every container, not just the daemon | Script defaults to stop-then-mask; `MODE=everything` keeps the old behaviour, documented | guru-bharadwaj20 | ✅ |
| `observe` broke on nested quotes over ssh | `remote()` sends the script on stdin; prints the shim count | guru-bharadwaj20 | ✅ |
| Both CNPG replicas on one node stall every write when that node dies | Add a third worker and set `podAntiAffinityType: required`; until then, check placement after every reschedule (`kubectl -n satark-data get pods -o wide`) and move a replica with a switchover or by deleting a replica pod | guru-bharadwaj20 | |
| Scan saving in the request path turns a DB stall into user timeouts | Same finding as GD-05: make `SaveScan` asynchronous (queue or background write), or return the verdict before persisting | guru-bharadwaj20 | |
| Pod and CNPG status stale for 5 min on a runtime-dead node | RB-002 section 3: check shims and processes on the node and `pg_stat_replication` on the primary, not pod status | guru-bharadwaj20 | ✅ |
| Alert timing untested | Re-run on the cloud cluster with Prometheus | guru-bharadwaj20 | |
