# RB-006: Backups (Velero, CNPG, etcd)

**Alerts:** `VeleroBackupFailed`, `VeleroBackupTooOld`, `CNPGLastFailedArchiveTime`,
`CNPGBackupTooOld`, `EtcdBackupTooOld` (from `satark_etcd_backup_last_success_timestamp_seconds`)

| What | Tool | Schedule | Where |
| --- | --- | --- | --- |
| Kubernetes objects + volumes | Velero (kopia FS backup) | daily 02:15 UTC, hourly objects-only | `s3://satark-backups-prod/velero` |
| Postgres | CNPG barman (WAL + base) | continuous WAL, daily base 01:30 UTC | `s3://satark-backups-prod/cnpg/satark-pg` |
| etcd | `etcd_backup` role, systemd timer | every 6 h | `s3://satark-backups-prod/etcd/<host>/` |

## 1. Velero backup failed or too old

```bash
velero backup get | head
velero backup describe <name> --details
velero backup logs <name> | grep -iE "error|warn" | head -40
kubectl -n velero get backupstoragelocation    # must be Available
kubectl -n velero get pods                      # velero + node-agent on every node
```

| Error | Fix |
| --- | --- |
| BSL `Unavailable`, `AccessDenied` | `velero-s3` Secret / IAM policy; re-sync External Secrets |
| `PartiallyFailed` with pod volume errors | node-agent missing on that node, or the pod was deleted mid-backup; re-run |
| `timed out waiting for ... to be ready` | large volume; raise `--fs-backup-timeout` on the server |
| Schedule not creating backups | `kubectl -n velero get schedule daily -o yaml` - check `status.phase` and `lastBackup` |

Re-run on demand: `velero backup create manual-$(date +%Y%m%d%H%M) --from-schedule daily --wait`.

## 2. CNPG backups

```bash
kubectl -n satark-data get backups.postgresql.cnpg.io --sort-by=.metadata.creationTimestamp | tail -3
kubectl -n satark-data get scheduledbackup satark-pg-daily
kubectl -n satark-data get cluster satark-pg -o jsonpath='{.status.firstRecoverabilityPoint}{"  "}{.status.lastSuccessfulBackup}{"\n"}'
```

WAL archiving failures: see RB-003 section 5. Base backup failures usually
share the same cause (credentials, network). On-demand:
`kubectl cnpg backup satark-pg -n satark-data`.

## 3. etcd snapshots

```bash
ssh cp1 systemctl list-timers etcd-backup.timer
ssh cp1 sudo journalctl -u etcd-backup.service -n 50 --no-pager
aws s3 ls s3://satark-backups-prod/etcd/cp1/ | tail -3
```

Run now: `ssh cp1 sudo systemctl start etcd-backup.service`.

### Restore etcd (control plane lost or corrupted)

Only with the incident commander's approval; this rewinds the cluster state.

```bash
aws s3 cp s3://satark-backups-prod/etcd/cp1/<file>.db.gz . && gunzip <file>.db.gz
sha256sum -c <file>.db.gz.sha256   # verify before gunzip if you kept the .gz
ssh cp1 sudo mv /etc/kubernetes/manifests/etcd.yaml /root/       # stop etcd
ssh cp1 sudo etcdutl snapshot restore /root/<file>.db --data-dir /var/lib/etcd-restore
# edit /root/etcd.yaml: hostPath for etcd-data -> /var/lib/etcd-restore
ssh cp1 sudo mv /root/etcd.yaml /etc/kubernetes/manifests/
```

Then restart the kubelet and verify `kubectl get nodes`. Argo CD will
re-sync anything newer than the snapshot from Git.

## 4. Restore tests

Backups that are never restored are not backups.

- PITR test: `tests/chaos/pitr/pitr-test.sh` (monthly).
- Velero restore into a scratch namespace (monthly):
  `velero restore create --from-backup <name> --include-namespaces satark --namespace-mappings satark:satark-restore-test`
- Full DR drill: SOP-05 (quarterly).

## Verify

- Newest Velero `daily-*` backup `Completed`, under 26 h old.
- CNPG `lastSuccessfulBackup` under 26 h; `ContinuousArchiving=True`.
- etcd metric `time() - satark_etcd_backup_last_success_timestamp_seconds < 7h`.
