# RB-003: Postgres (CNPG `satark-pg`)

**Alerts:** `CNPGClusterHAWarning`, `CNPGClusterNotHealthy`, `CNPGLastFailedArchiveTime`,
`SatarkDBConnectionErrors`, `PostgresReplicationLagHigh`
**Namespace:** `satark-data` · **Cluster:** `satark-pg` · **Services:** `satark-pg-rw`, `satark-pg-ro`

## 1. Triage (2 minutes)

```bash
kubectl -n satark-data get cluster satark-pg
kubectl -n satark-data get pods -l cnpg.io/cluster=satark-pg -L cnpg.io/instanceRole -o wide
kubectl cnpg status satark-pg -n satark-data      # if the cnpg plugin is installed
```

Decide which case you are in:

| Symptom | Go to |
| --- | --- |
| No pod has role `primary`, or phase "Failing over" for > 5 min | 2 |
| Primary fine, a replica not Ready or lagging | 3 |
| Apps report `too many clients` / connection refused | 4 |
| `ContinuousArchiving` condition False / WAL archive alert | 5 |
| PVC full (`no space left on device` in logs) | 6 |

## 2. No primary / failover stuck

1. Check the operator is running and read its log:
   `kubectl -n cnpg-system logs deploy/cnpg-cloudnative-pg --since=15m | grep satark-pg`
2. If a healthy replica exists and the operator is waiting on the old
   primary's node (node NotReady), promote manually:
   `kubectl cnpg promote satark-pg satark-pg-2 -n satark-data`
3. If every instance is down and PVCs are intact, restart the instances one
   at a time: `kubectl -n satark-data delete pod satark-pg-1`.
4. If data is lost on all instances, restore from backup: follow the recovery
   bootstrap in `tests/chaos/dr/satark-pg-recovery.yaml` into a **new** cluster
   name, verify, then repoint apps (SOP-05). Never delete the old cluster
   until the new one is verified.

## 3. Replica unhealthy or lagging

- Lag: `kubectl cnpg status` shows replay lag. Short spikes during bulk
  writes are normal. Sustained lag usually means disk or network on the
  replica node: check `kubectl describe node` and node-exporter disk latency.
- Replica crash-looping with `requested WAL segment has already been removed`:
  delete the replica's PVC and pod; CNPG rebuilds it from the primary:
  ```bash
  kubectl -n satark-data delete pvc satark-pg-2 && kubectl -n satark-data delete pod satark-pg-2
  ```

## 4. Connection exhaustion

```bash
kubectl -n satark-data exec satark-pg-1 -c postgres -- psql -c \
  "select usename, application_name, state, count(*) from pg_stat_activity group by 1,2,3 order by 4 desc"
```

- One service holding most connections: restart that Deployment and lower
  its pool size.
- Idle-in-transaction sessions: terminate them
  (`select pg_terminate_backend(pid) from pg_stat_activity where state='idle in transaction' and now()-state_change > interval '5 min'`).
- Long term: put a CNPG `Pooler` (PgBouncer) in front of `satark-pg-rw`.

## 5. WAL archiving failing

```bash
kubectl -n satark-data get cluster satark-pg -o jsonpath='{.status.conditions[?(@.type=="ContinuousArchiving")]}'
kubectl -n satark-data logs satark-pg-1 -c postgres | grep -i barman | tail -20
```

- `AccessDenied` / `InvalidAccessKeyId`: the `satark-pg-s3` Secret is wrong or
  the key was rotated. Re-sync External Secrets and check the IAM policy.
- Network: test from the pod `curl -sI https://s3.ap-south-1.amazonaws.com`.
- **Urgency:** WAL piles up in `pg_wal` until archiving succeeds and can fill
  the disk (case 6). RPO grows for every minute this is broken.

## 6. Volume full

1. Check usage: `kubectl -n satark-data exec satark-pg-1 -c postgres -- df -h /var/lib/postgresql/data`.
2. If `pg_wal` is large, fix archiving first (5).
3. Grow the volume (storage class must allow expansion): edit
   `spec.storage.size` on the Cluster in Git and sync. CNPG resizes PVCs
   one instance at a time.

## Verify

- `kubectl cnpg status` shows one primary, all replicas streaming, archiving OK.
- App error rate back to baseline.
- Take an on-demand backup after any failover:
  `kubectl cnpg backup satark-pg -n satark-data`.
