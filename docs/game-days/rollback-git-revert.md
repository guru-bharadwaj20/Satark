# Game day: roll back with `git revert`

**Goal:** prove that undoing a bad release is one Git operation, and measure
how long it takes from `git revert` to a healthy rollout.

**Status:** not run yet. Fill in the results table after the drill.

## Setup

1. Pick a service in `dev`, for example `gateway-api`.
2. Ship a "bad" release: set `image.tag` in
   `gitops/envs/dev/apps/gateway-api/values.yaml` to a tag that starts but
   fails readiness (or a real previous build), commit and push. Note the SHA.
3. Wait until Argo CD shows `gateway-api-dev` Synced (and Degraded).

## Run

```bash
scripts/rollback-drill.sh <bad-commit-sha> gateway-api dev
```

The script reverts the commit, pushes it, triggers an Argo CD refresh, and
waits until the Deployment runs the previous tag and the rollout finishes. It
appends a row to `rollback-timings.csv`.

## Results

| Date (UTC) | Service | Env | Push (s) | Argo CD sync (s) | Rollout (s) | Total (s) |
|---|---|---|---|---|---|---|
| _pending_ | | | | | | |

Target: total under 5 minutes without a manual refresh, under 1 minute with
the refresh the script sends.

## Notes

- Without the refresh, sync time is bounded by `timeout.reconciliation`
  (180 s in `gitops/platform/argocd/argocd-cm.yaml`). A GitHub webhook to
  Argo CD would remove that wait.
- Record anything that surprised you here.
