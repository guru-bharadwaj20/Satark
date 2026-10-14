# GD-01: Lose a worker node

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
| Script | `tests/chaos/gd-01-lose-worker.sh` |
| Expected duration | 45 minutes |

## Hypothesis

When `worker-2` is powered off without a drain:

- `KubeNodeNotReady` fires within 2 minutes.
- Pods on the node are marked for deletion after the default 300 s
  `node.kubernetes.io/unreachable` toleration and rescheduled on `worker-1`.
- Stateless services with 2+ replicas and a PDB see no user-facing errors
  beyond brief retries; single-replica services are unavailable for up to
  ~6 minutes.
- If the CNPG primary was on `worker-2`, CNPG promotes the replica within
  about 60 s (see GD-02).

## Blast radius and abort criteria

- Scope: one of two workers; on-prem cluster only.
- Abort if the public probe fails continuously for more than 10 minutes, or
  `worker-1` shows memory pressure.
- Abort action: `tests/chaos/gd-01-lose-worker.sh revert`.

## Procedure

1. `PROBE_URL=https://api.satark.example/healthz tests/chaos/gd-01-lose-worker.sh observe`
2. `... inject` (starts a 10 minute probe)
3. Responder works from the `KubeNodeNotReady` alert. Expected path:
   `kubectl get nodes`, `kubectl satark doctor`, RB-001 / RB-003.
4. Record when pods were rescheduled: `kubectl get events -A --field-selector reason=TaintManagerEviction`.
5. `... revert` and confirm pods rebalance.

## What to look for

- Did any Deployment have all replicas on the lost node (missing
  topology spread constraints)?
- Did PVC-backed pods get stuck in `ContainerCreating` because local volumes
  cannot move? Which services depend on node-local storage?
- Did Valkey or NATS lose quorum or data?

## Results

| Measure | Value |
| --- | --- |
| Time to alert | |
| Time to diagnose | |
| Pods rescheduled after | |
| Probe failures / total | |
| Hypothesis held? | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
