# SOP-05: Disaster recovery drill

| | |
| --- | --- |
| Owner | Platform on-call (drill lead) plus one observer |
| Frequency | Quarterly, and after any change to backups or the data layer |
| Duration | 2 to 3 hours including cleanup |
| Targets | RTO 2 h, RPO 15 min |
| Automation | `scripts/dr-drill.sh`, `infra/terraform/envs/dr` |

The same steps are used in a real disaster. In a drill, production keeps
running and only DNS is moved briefly.

## 1. Roles

- **Drill lead:** runs the commands, calls out each phase.
- **Observer/scribe:** keeps the timeline, notes every surprise, keeps time.

## 2. Preconditions

- [ ] Latest Velero `daily-*` backup is `Completed` and under 26 hours old:
      `velero backup get | head`.
- [ ] Latest CNPG backup succeeded and WAL archiving is healthy:
      `kubectl -n satark-data get cluster satark-pg -o jsonpath='{.status.conditions[?(@.type=="ContinuousArchiving")].status}'`
      returns `True`.
- [ ] Operator laptop has `terraform`, `aws` (with drill IAM role), `kubectl`,
      `helm`, `velero`, `jq`.
- [ ] The DNS record you will move has a TTL of 60 seconds (lower it a day
      in advance if not).
- [ ] Drill announced in the status channel with the window.

## 3. Run

1. Declare the start of the simulated outage. Note the time; the script uses
   it for RTO and RPO:
   ```bash
   export OUTAGE_START=$(date -u +%s)
   ```
2. Run the drill:
   ```bash
   ADMIN_CIDR=$(curl -s https://checkip.amazonaws.com)/32 \
   HOSTED_ZONE_ID=<zone id> RECORD=api.satark.example \
   scripts/dr-drill.sh
   ```
   For a drill, use a dedicated record such as `dr-test.satark.example`
   instead of the production name so users are not moved.
3. The script prints each phase. If a phase fails, **do not restart from
   scratch**: fix the cause and continue from that phase by hand using the
   commands in the script. Note the fix in the timeline.

## 4. Verify

From a network outside the cluster:

```bash
curl -s https://$RECORD/healthz
curl -s -XPOST https://$RECORD/v1/check/text -H 'content-type: application/json' \
  -d '{"text":"Your KYC is pending, click http://example.test now","lang":"en"}'
```

Then work through the checklist in `docs/game-days/dr-drill.md`.

## 5. Fail back and clean up

1. Point DNS back at the edge proxy Elastic IP (or remove the test record).
2. Wait one TTL and confirm traffic is on-prem again (`satark_http_requests_total`
   rising in on-prem Grafana).
3. Destroy the DR environment:
   ```bash
   terraform -chdir=infra/terraform/envs/dr destroy -var 'admin_cidrs=["0.0.0.1/32"]'
   ```
4. Delete the local kubeconfig in `docs/game-days/dr-runs/<run>/`.
5. In a real disaster, before failing back, take a fresh Velero backup and
   CNPG backup on the DR cluster so writes made during the outage are kept,
   and restore them on-prem.

## 6. Record

- Fill in `docs/game-days/dr-drill.md` (RTO, RPO, timeline, problems).
- File an issue for every problem that added more than 5 minutes to RTO.
- If RTO or RPO missed target, schedule a repeat drill within 30 days.
