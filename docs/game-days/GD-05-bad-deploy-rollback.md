# GD-05: Ship a bad deploy and roll back

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab) |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
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

Lab: `gateway-api` in `satark-dev`, managed by the Argo CD Application
`gateway-api-dev` (auto-sync with self-heal, tracking `main`):
`NS=satark-dev ARGO_APP=gateway-api-dev`. The probe sent
`POST /v1/scan/text` once a second with a 2 s client timeout. A second,
single probe ran only during each recovery.

The GitOps variant was timed as "turn self-heal back on and let Argo CD put
the Git version back". The bad change lived only in the cluster, as an
out-of-band hotfix would, so there was no commit to revert. A deliberately
broken commit was not pushed to `main`.

| Measure | `rollout undo` | GitOps (self-heal back on) |
| --- | --- | --- |
| Time to alert | n/a (no Prometheus in the lab) | n/a |
| Time to identify the bad release | under 1 min: Argo CD showed `OutOfSync`, `rollout history` showed `gd-05 bad release`, gateway log named port 9999 | same |
| Bad rollout "succeeded"? | yes, 12 s, both pods Ready | yes, 13 s |
| Time to recover | 9 s to the first 200, rollout done in 12 s | 12 s to the first 200 (Argo CD self-heal) |
| Failed requests while bad | 300 / 300 (all client timeouts) | 300 / 300 |

Raw logs: `logs/gd-05-bad-deploy-20261018.log`, and per-variant traces
`logs/gd-05-bad-deploy-{fast,gitops}-trace-20261018.log` (bad window) and
`...-recovery-trace-20261018.log` (recovery).

Caveat: the Windows host was running heavy WSL jobs alongside the lab VMs.
It probably caused the unrelated incident below.

## Timeline (UTC)

| Time | Event |
| --- | --- |
| 14:55:14 | Auto-sync paused; `TEXT_ANALYZER_URL` set to port 9999 |
| 14:55:26 | Bad rollout reports success; Argo CD `OutOfSync Healthy` |
| 14:55:28 → 15:10:49 | Every text scan times out |
| 15:10:49 | `rollout undo` |
| 15:10:58 | First 200 |
| 15:11:01 | Rollout complete; auto-sync re-enabled, app `Synced Healthy` |
| 15:11:07 → 15:12:21 | Unrelated: Postgres sync replication stall (below) |
| 15:12:51 | GitOps variant: same bad change |
| 15:13:04 → 15:28:26 | Every text scan times out |
| 15:28:26 | Self-heal re-enabled |
| 15:28:38 | First 200; no failures after |

## What we learned

- The hypothesis held: `/readyz` does not check dependencies, so a release
  pointing at a wrong port rolls out green. Only the error rate and Argo
  CD's `OutOfSync` showed it.
- A wrong port fails slowly. Nothing listens on `text-analyzer:9999`, and
  with Cilium's kube-proxy replacement an unknown Service port is dropped,
  not refused. Every call waited the full 2 s `ANALYZER_TIMEOUT`, so a
  client with a 2 s timeout saw a hang rather than a quick 503.
- Both mitigations were fast here: 9–12 s. In this lab the GitOps path cost
  nothing extra, because Argo CD already knew the good state; re-enabling
  self-heal *is* the rollback. `rollout undo` leaves the app on a revision
  Git does not describe until auto-sync is back on. In this run the two
  happened to match.
- The script left `kubernetes.io/change-cause: gd-05 bad release` on the
  Deployment, and Kubernetes copied it onto the *good* revision when Argo
  CD restored it. `rollout history` then labelled the good revision as the
  bad release. The script now removes the annotation before rolling back.
- **Unrelated incident found during the run.** At 15:11:07 both Postgres
  replicas, which sit on worker-1, dropped their replication connections
  (`terminating walsender process due to replication timeout`), most
  likely during a host CPU stall. With `minSyncReplicas: 1`, every commit
  then waited for a standby. Text scans still returned 200 but took
  exactly 2 s: `gateway-api` saves each scan to Postgres *inside the
  request*, bounded by `store.WriteTimeout` (2 s). Postgres logged
  `canceling wait for synchronous replication due to user request` 15
  times. Those rows are committed on the primary but were not confirmed on
  a standby when the API answered. The standbys reconnected at 15:12:21.
  Three things combine here: both replicas on one node, synchronous
  replication, and a blocking save in the request path.

## Questions answered

- *Would an Argo Rollouts canary have stopped this?* Yes. Every request on
  the new version failed, so any error-rate analysis would abort on the
  first step. It would cost a Rollout CRD per service and an analysis
  template wired to Prometheus. That is worth it for `gateway-api` only once
  Prometheus is in the loop.
- *Should `/readyz` check dependencies?* Checking that `TEXT_ANALYZER_URL`
  accepts a TCP connection at startup would have failed this rollout in
  seconds. A full dependency check on every probe would instead take
  every gateway pod out together when text-analyzer blips. The recommendation
  is a startup-only check.
- *Deploy events on Grafana:* not testable in the lab (no Grafana).

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| change-cause annotation mislabelled the good revision | `gd-05-bad-deploy.sh` removes it before rollback/revert | guru-bharadwaj20 | ✅ |
| Bad dependency URL rolls out green | Startup check in `gateway-api` that each analyzer URL accepts a connection before reporting ready | guru-bharadwaj20 | |
| A Postgres stall adds 2 s to every scan | Move `SaveScan` off the request path (bounded async queue), or shorten its timeout for scans | guru-bharadwaj20 | |
| Both CNPG replicas on one worker; one node blip stalls sync commits | Lab has only two workers, so it is unavoidable here; on three or more workers require replica anti-affinity (`affinity.podAntiAffinityType: required`) | guru-bharadwaj20 | |
| Alert timing (`SatarkHighErrorRate`) untested | Re-run on the cloud cluster with Prometheus | guru-bharadwaj20 | |
