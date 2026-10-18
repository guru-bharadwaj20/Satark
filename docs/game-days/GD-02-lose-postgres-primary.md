# GD-02: Lose the Postgres primary

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab) |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
| Script | `tests/chaos/gd-02-lose-pg-primary.sh` |
| Runbook under test | RB-003 (Postgres) |
| Expected duration | 45 minutes |

## Hypothesis

- CNPG promotes the replica within 60 s of the primary pod being deleted.
- The `satark-pg-rw` Service follows the new primary automatically; apps
  reconnect with no config change.
- Writes fail for at most ~30 s (`gateway-api` returns 503 for report
  submissions; text checks keep working because they do not need Postgres).
- No committed transaction is lost (synchronous replication off: up to a few
  seconds of WAL may be at risk; measure it).
- With `--with-pvc`, the old primary is rebuilt as a replica via
  `pg_basebackup` and the cluster returns to full strength.

## Blast radius and abort criteria

- Scope: the single production database cluster.
- Abort if no new primary within 5 minutes, or both instances are not Ready.
- Abort action: follow RB-003 "No primary", restore from backup if needed.

## Procedure

1. Start a write loop in another terminal to measure write loss:
   ```bash
   kubectl -n satark-data exec -it satark-pg-1 -- psql -d app -c \
     "CREATE TABLE IF NOT EXISTS gd02 (n bigserial, at timestamptz default now())"
   while true; do kubectl -n satark-data exec svc/satark-pg-rw -- psql -d app -Atc \
     "INSERT INTO gd02 DEFAULT VALUES RETURNING n" ; sleep 0.5; done
   ```
2. `tests/chaos/gd-02-lose-pg-primary.sh observe`
3. `... inject`, later repeat with `--with-pvc`.
4. Responder diagnoses from `CNPGClusterHAWarning` / app error alerts.
5. `... revert` (waits for all instances), then compare the last `n`
   acknowledged by the loop with `max(n)` in the table.

## Results

Lab: kubeadm v1.35 on three VirtualBox VMs (2 vCPU / 4 GB each), CNPG
cluster `satark-pg` with 3 instances and `minSyncReplicas: 1`. The write
loop ran `INSERT ... RETURNING n` against `satark-pg-rw` every 0.5 s from a
replica pod (database `satark`, not `app` as in the procedure above). The
probe hit `POST /v1/scan/text` through the gateway every second.

| Measure | Pod delete | Pod + PVC delete |
| --- | --- | --- |
| Failover time | 3 s (pg-1 → pg-2) | 4 s (pg-2 → pg-1) |
| Write errors seen by loop | 4 (9.6 s gap) | 4 (9.6 s gap) |
| Acknowledged rows lost | 0 of 311 | 0 of 229 |
| Probe failures (text scan) | 1 / 180 | 1 / 180 |
| Time to full strength | 3 min 23 s | **never on its own**; 31 s after the manual fix |
| Time to detect | n/a (no Prometheus in the lab) | n/a; found by hand 11 h 45 min later |
| Time to diagnose | n/a | ~1 min from `kubectl describe pod` |

Raw logs: `logs/gd-02-lose-pg-primary-20261018.log`,
`logs/gd-02-writes-pod-20261018.log`, `logs/gd-02-writes-pvc-20261018.log`,
plus the doctor snapshots.

Caveat: the Windows host was also running heavy WSL jobs, so the VMs were
CPU-starved at times. Timings are likely pessimistic.

## Timeline (UTC)

| Time | Event |
| --- | --- |
| 02:24:05 | Variant 1: force-delete primary pod `satark-pg-1` |
| 02:24:09 | `satark-pg-2` promoted; writes resume at 02:24:15 |
| 02:27:28 | All 3 instances Ready again |
| 02:28:50 | Variant 2: delete PVC `satark-pg-2` and force-delete its pod |
| 02:28:54 | `satark-pg-1` promoted; writes resume at 02:29:00 |
| 02:28:54 → 14:14 | New `satark-pg-2` pod `Pending`: `persistentvolumeclaim "satark-pg-2" not found`. The cluster runs on 2 of 3 instances |
| 14:14:51 | Fix: delete leftover PVC `satark-pg-2-wal` and the Pending pod |
| 14:15:22 | CNPG builds `satark-pg-4` from the primary; cluster healthy |

## What we learned

- Failover is fast and safe: 3–4 s to a new primary, about 10 s of failed
  writes, and no acknowledged write lost in either run. The 30 s / 60 s
  hypotheses hold with room to spare. The hypothesis assumed async
  replication; the cluster actually runs `minSyncReplicas: 1`, which is why
  no acknowledged row was lost.
- The text-scan path barely noticed (1 failed probe each time, a 2 s timeout),
  as the hypothesis said: it does not need Postgres.
- **Losing a disk does not self-heal.** The cluster has a separate WAL volume.
  Deleting only the data PVC leaves `satark-pg-2-wal` behind, so CNPG still
  treats instance 2 as existing and recreates its pod against a claim that
  is gone. It stayed Pending for almost 12 hours with the cluster one
  replica short and `minSyncReplicas: 1` at risk if another instance failed.
  RB-003 told responders to do exactly this (delete only the data PVC).
- Nothing told us. In production `CNPGClusterHAWarning` should fire on
  `readyInstances < instances`; the lab has no Prometheus, so this run
  could not test it.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| Deleting only the data PVC leaves the instance stuck Pending | RB-003 now deletes both `<instance>` and `<instance>-wal` PVCs and explains the stuck state | guru-bharadwaj20 | ✅ |
| Game-day script reproduced the half-delete | `gd-02-lose-pg-primary.sh --with-pvc` deletes the `-wal` PVC too | guru-bharadwaj20 | ✅ |
| No signal that the cluster sat at 2/3 for 12 h | Check that `CNPGClusterHAWarning` fires within 5 min, on the cloud cluster with Prometheus | guru-bharadwaj20 | |
