# Data retention

Satark keeps as little as it can, for as short a time as it can. This table is the policy; the privacy policy (`docs/privacy-policy.md`) is the user-facing summary.

| Data | Where | Kept for | How it's removed |
|---|---|---|---|
| Community reports (hashed subject, hashed reporter, category, time) | `reputation_reports` (satark-pg) | 365 days | `reputation-retention` CronJob, daily |
| Delisting requests (masked subject, reason) | `reputation_delistings` | 365 days after decision | same CronJob |
| Requester contact on a delisting request | `reputation_delistings.contact` | 30 days after decision | same CronJob blanks it |
| A user's reports | `reputation_reports` | until `/forget` | `POST /v1/forget` from the bot |
| Text/links submitted for a check | not stored; only redacted text reaches the LLM | request lifetime | n/a |
| Sandbox screenshots | object storage | 7 days | bucket lifecycle rule |
| Logs | Loki | 14 days | Loki retention |
| Backups | object storage | 30 days | backup retention; deleted data ages out of backups within 30 days |

## The CronJob

`apps/reputation/deploy/retention-cronjob.yaml` runs `reputation retention` daily at 02:17 UTC. It deletes in one transaction and logs counts only, never identifiers.

Run it by hand:

```bash
kubectl -n satark create job --from=cronjob/reputation-retention retention-manual-$(date +%s)
kubectl -n satark logs -l app.kubernetes.io/component=retention --tail=5
```

## Verify

```bash
# The last run succeeded
kubectl -n satark get jobs -l app.kubernetes.io/component=retention --sort-by=.metadata.creationTimestamp | tail -2

# Nothing older than the window remains (expect 0)
kubectl -n satark-data exec -i satark-pg-1 -c postgres -- psql -d satark -tAc \
  "SELECT count(*) FROM reputation_reports WHERE created_at < now() - interval '366 days'"
```
