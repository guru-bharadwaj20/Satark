# PITR test results

Status: **executed on 2026-10-23 (on-prem lab). PASS.** Task 11.2.
Re-run with `tests/chaos/pitr/onprem/setup.sh`, then `tests/chaos/pitr/pitr-test.sh`
(see the header of `setup.sh` for the variables), then `setup.sh --teardown`.

| Field | Value |
| --- | --- |
| Date (UTC) | 2026-10-23, 14:02 to 14:08 |
| Operator | guru-bharadwaj20 |
| Source cluster / backup ID | satark-pg (3 instances, Postgres 16.4, CNPG 1.24.1) / `20261023T140203` (from a standby, WAL `…53` to `…54`, 9 s) |
| Object store | SeaweedFS 4.48 in the cluster (`satark-backup` namespace), standing in for S3 |
| Recovery target time | 2026-10-23 14:05:53.407803+00 |
| Restore duration (s) | 37 (new cluster created to Ready) |
| Marker row present | yes (`pitr-1792764349`, written 14:05:51) |
| Post-target row absent | yes (`pitr-1792764349-bad`, written 14:05:55) |
| Result | PASS |

The restored cluster held every row up to the target and none after it, so
for a chosen target time no committed data before that time was lost. The
RPO for an unplanned loss is the WAL not yet archived: `archive_timeout` is
5 min, and in this run the last segment reached the store within 60 s of the
WAL switch.

## Notes

- **Base backup in an unarchived segment.** The first attempt failed with
  `WAL not found 000000090000000000000050`. Segments closed just before
  the backup config reached the instance were reported as archived without
  being uploaded, and the base backup started in one of them. `setup.sh` now
  switches WAL and waits for a segment from after the change before it takes
  the base backup. The same applies when backups are first turned on in AWS:
  check that a new segment is in the bucket before relying on the first
  backup.
- **`recovery_target_time` format.** `pitr-test.sh` passed `…T14:02:25Z`.
  CNPG 1.24.1 rewrote it as `14:02:25.000000Z`, which Postgres rejects
  (`invalid value for parameter "recovery_target_time"`), and recovery
  never started. The script now passes `YYYY-MM-DD HH24:MI:SS.US+00`.
- **Other script fixes.** The database name is configurable (`DB`, the
  lab uses `satark`), the restore uses the source's image and storage
  class, `ENDPOINT_URL` points at a non-AWS store, and the last archived
  WAL is read from `pg_stat_archiver` (the cluster status field was empty).
- **Operator restarts.** The CNPG operator lost its leader lease when the
  lab API server was slow (90 restarts in 4 days) and its webhook refused
  one create. Lab load, not a recovery problem; the retry succeeded.
- MinIO no longer publishes container images, so the lab store is SeaweedFS.
- The test table `pitr_probe` was dropped from `satark-pg` afterwards and
  the backup config, Secret and store were removed (`setup.sh --teardown`).
- Evidence: `docs/game-days/logs/pitr-20261023-*`.

## Actions

| Action | Owner | Done |
| --- | --- | --- |
| Fix the target-time format and DB name in `pitr-test.sh` | guru-bharadwaj20 | ✅ |
| Wait for a post-change WAL segment before the first base backup | guru-bharadwaj20 | ✅ (lab script) |
| Run the same test against S3 once the AWS account exists (RB-006, monthly) | guru-bharadwaj20 | |
