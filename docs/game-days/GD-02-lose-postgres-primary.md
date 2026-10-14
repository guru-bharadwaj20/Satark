# GD-02: Lose the Postgres primary

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
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

| Measure | Pod delete | Pod + PVC delete |
| --- | --- | --- |
| Failover time | | |
| Write errors seen by loop | | |
| Acknowledged rows lost | | |
| Time to full strength | | |
| Time to diagnose | | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
