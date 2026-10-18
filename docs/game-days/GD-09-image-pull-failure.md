# GD-09: Image pull failure

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab) |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
| Script | `tests/chaos/gd-09-image-pull.sh` |
| Runbook under test | RB-001 (CrashLoop / ImagePull) |
| Expected duration | 40 minutes (two variants) |

## Hypothesis

**Variant A, non-existent tag:**
- The rolling update creates one new pod that sits in `ErrImagePull` /
  `ImagePullBackOff`. With `maxUnavailable: 0` the old pods keep serving, so
  there is **no user impact**, only a stuck rollout.
- `KubeDeploymentRolloutStuck` fires after `progressDeadlineSeconds` (600 s).
  `kubectl satark doctor` shows the pod and the stuck deployment.

**Variant B, registry unreachable from one node:**
- Pods already running are unaffected. A pod rescheduled to the node fails
  to pull while the same image pulls fine elsewhere, which points to a node
  problem rather than a bad tag.
- If the Deployment has `imagePullPolicy: Always`, even cached images fail.

## Blast radius and abort criteria

- Scope: one Deployment (`text-analyzer`). Abort if old replicas start
  failing too (would mean `maxUnavailable` is not 0).
- Abort action: `... revert`.

## Procedure

1. `MODE=tag ... inject`; responder diagnoses from doctor or the rollout
   alert (`kubectl describe pod` shows `manifest unknown`); `... revert`.
2. `MODE=registry ... inject`; responder diagnoses (`dial tcp 0.0.0.0:443`
   in events, only on one node); `MODE=registry ... revert`.

## Things to check

- Is every Deployment's rollout strategy `maxUnavailable: 0`?
- Does CI verify the image tag exists before the GitOps commit is made?
- Do we pin images by digest in production?

## Results

Lab: `text-analyzer` in `satark-dev` (`NS=satark-dev APP=text-analyzer`,
plain Helm release, `maxSurge: 1`, `maxUnavailable: 0`,
`progressDeadlineSeconds: 600`, `imagePullPolicy: IfNotPresent`). The probe
sent `POST /v1/scan/text` once a second; that path needs text-analyzer.

The lab images are side-loaded (`:lab` tags are not in ghcr.io). For variant
B, the image was therefore exported from worker-2 before the script removed
it, and imported back before the revert. That stands in for "the registry
serves it again". Both text-analyzer pods were on worker-2 at the start.

| Measure | Variant A (bad tag) | Variant B (registry blocked on worker-2) |
| --- | --- | --- |
| User-facing errors | 0 / 180 | 4 / 180 (`503` for 4 s right after the inject) |
| Time to alert | n/a (no Prometheus); `ProgressDeadlineExceeded` set at 10 m 02 s, and `kubectl satark doctor` reported FAIL ("rollout stuck") | n/a |
| Time to diagnose | under 1 min: `ErrImagePull ... v0.0.0-doesnotexist` in events | under 1 min: same image Running on worker-1, `ErrImagePull` on worker-2 only |
| Old pods kept serving | yes, both, the whole time | n/a (the script deletes them) |

Raw logs: `logs/gd-09-image-pull-20261018.log`, probe trace
`logs/gd-09-image-pull-registry-trace-20261018.log`.

Caveat: the Windows host was running heavy WSL jobs alongside the lab VMs.

## Timeline (UTC)

| Time | Event |
| --- | --- |
| 15:30:28 | A: image set to `:v0.0.0-doesnotexist`; one surge pod on worker-1 goes `ErrImagePull` → `ImagePullBackOff` |
| 15:30:28 → 15:40:58 | Both old pods serve; 0 failed probes |
| 15:40:30 | `Progressing=False`, `ProgressDeadlineExceeded` |
| 15:40:58 | Revert: image restored, rollout completes |
| 15:41:22 | B: `ghcr.io` black-holed in worker-2's `/etc/hosts`, cached image removed, worker-2's text-analyzer pods deleted |
| 15:41:24 → 15:41:27 | 4 × 503: both replicas were on worker-2, so the API had no text-analyzer until the replacement on worker-1 was Ready |
| 15:41:36 | Replacements: worker-1 Running (cached image), worker-2 `ErrImagePull` |
| 15:44:44 | Revert ran, but its remote `sed` failed (see below), leaving the black-hole in place |
| 15:48:20 | Fixed by hand; stuck pod deleted; 2/2 Ready, one per worker |

## What we learned

- **Variant A** matched the hypothesis exactly. `maxUnavailable: 0`
  means a bad tag costs nothing but a stuck rollout. The deadline fired at
  600 s, and the doctor flagged both the pod and the stuck Deployment.
  `KubeDeploymentRolloutStuck` would key off the same condition.
- **Variant B** showed the node-local signature the hypothesis described:
  the same image Running on one node and failing to pull on another
  points at the node or its path to the registry, not at the tag.
- The 4 s outage in B came from placement, not from the registry. Both
  replicas had ended up on worker-2. The chart's spread constraint is
  `ScheduleAnyway`, so skew is allowed, and losing that node's pods took
  out the service. After the reschedule they were spread one per node.
- **Script bug:** the revert ran `$SSH node -- sudo sed -i "/ ghcr.io$/d" /etc/hosts`.
  ssh (and `lab-ssh.sh`, which mimics it) joins its arguments into one
  remote command line, so the quotes vanished. sed got `/` and
  `ghcr.io$/d` and failed, and `set -e` stopped the revert before the
  rollout check. With real ssh this would have left a production node
  unable to pull from ghcr.io after the game day. The remote command is
  now quoted for the remote shell, and the fix was tested on worker-2.
  The other `$SSH` call sites already pass one quoted string or a heredoc.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| Revert left the registry black-holed (ssh argument joining) | `gd-09-image-pull.sh` quotes the remote `sed`; tested | guru-bharadwaj20 | ✅ |
| Both replicas of a 2-replica service on one node | Consider `whenUnsatisfiable: DoNotSchedule` with `nodeTaintsPolicy: Honor` for the request-path services (the Envoy proxy already uses it), or a descheduler to rebalance | guru-bharadwaj20 | |
| Alert timing untested | Re-run on the cloud cluster with Prometheus (`KubeDeploymentRolloutStuck`) | guru-bharadwaj20 | |
