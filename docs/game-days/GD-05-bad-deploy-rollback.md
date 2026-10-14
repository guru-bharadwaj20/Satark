# GD-05: Ship a bad deploy and roll back

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
| Script | `tests/chaos/gd-05-bad-deploy.sh` |
| Expected duration | 40 minutes |

## Hypothesis

- A `gateway-api` release with a wrong dependency URL rolls out successfully
  because `/readyz` does not check dependencies.
- `SatarkHighErrorRate` (5xx ratio > 5% for 5 minutes) fires within 7 minutes.
- The responder links the alert to the most recent change (Argo CD history,
  `kubectl rollout history`) and rolls back in under 10 minutes from alert.
- Rollback via `kubectl rollout undo` is fast but creates drift; the durable
  fix is a Git revert so Argo CD converges on the good version.

## Blast radius and abort criteria

- Scope: all text checks through the API. Abort after 15 minutes of errors.
- Abort action: `... rollback` then `... revert`.

## Procedure

1. `... observe`
2. `... inject`
3. Responder diagnoses. Two acceptable mitigations, time both on separate runs:
   - **Fast:** `tests/chaos/gd-05-bad-deploy.sh rollback` (`kubectl rollout undo`).
   - **GitOps:** `argocd app rollback gateway-api <previous id>` or revert the
     commit in Git and sync.
4. `... revert` to restore auto-sync and confirm the app is Synced/Healthy.

## Questions to answer

- Would an Argo Rollouts canary with an error-rate analysis have stopped this
  automatically? What would it cost?
- Should `/readyz` check that required dependencies resolve and answer?
- Is the deploy event annotated on the Grafana dashboard?

## Results

| Measure | `rollout undo` | GitOps revert |
| --- | --- | --- |
| Time to alert | | |
| Time to identify the bad release | | |
| Time to recover | | |
| Failed requests | | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
