# GD-09: Image pull failure

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
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

| Measure | Variant A | Variant B |
| --- | --- | --- |
| User-facing errors | | |
| Time to alert | | |
| Time to diagnose | | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
