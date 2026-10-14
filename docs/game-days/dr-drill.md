# DR drill: Velero restore and DNS switch

Status: **template, not yet executed.** Run `scripts/dr-drill.sh` following
SOP-05, then copy `docs/game-days/dr-runs/<run>/summary.txt` and
`timeline.txt` here.

## Objectives

| Objective | Target | Actual |
| --- | --- | --- |
| RTO (outage declared to first good public request) | 2 h | |
| RPO (outage declared minus newest restored row) | 15 min | |

## Inputs

| Field | Value |
| --- | --- |
| Date (UTC) | |
| Participants | |
| Velero backup used | |
| Backup start time | |
| CNPG WAL replayed to | |

## Timeline

| Phase | Start (UTC) | Duration |
| --- | --- | --- |
| Provision VM (Terraform) | | |
| k3s ready | | |
| Velero restore | | |
| Postgres recovery from WAL | | |
| Apps Available | | |
| DNS switched | | |
| Service restored | | |

## Checks after restore

- [ ] `POST /v1/check/text` returns a verdict
- [ ] Telegram bot answers a test message
- [ ] Row counts in `scans` and `reports` within RPO of production
- [ ] Screenshots bucket reachable (read-only)
- [ ] Grafana Agent / metrics flowing (optional on DR)

## What went wrong

| Problem | Impact on RTO | Fix | Owner |
| --- | --- | --- | --- |

## Cleanup

- [ ] DNS pointed back at the edge proxy
- [ ] `terraform -chdir=infra/terraform/envs/dr destroy`
- [ ] Kubeconfig in `dr-runs/<run>/` deleted
