# GD-06: Container runtime failure

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
| Script | `tests/chaos/gd-06-runtime-failure.sh` |
| Expected duration | 40 minutes |

## Hypothesis

With containerd stopped and masked on `worker-1`:

- Existing containers keep serving for a while (shims are independent of
  the daemon), so user impact is delayed and partial.
- Within ~3 minutes the kubelet reports `PLEG is not healthy` /
  `container runtime is down` and the node becomes `NotReady`.
- After the 300 s unreachable toleration, pods are evicted and recreated on
  other nodes; old containers on `worker-1` keep running as orphans until
  containerd returns and the kubelet kills them.
- Liveness probes on `worker-1` fail to restart anything (no runtime), so
  any container that crashes in this window stays dead.

The interesting part is the difference from GD-01: the node is reachable by
SSH and still answers some traffic, which can mislead the responder.

## Blast radius and abort criteria

- Scope: one worker. Abort if the other worker shows pressure or the probe
  fails for more than 10 minutes.
- Abort action: `... revert`.

## Procedure

1. `... observe`, then `... inject`.
2. Responder starts from `KubeNodeNotReady`. Expected path: `kubectl describe
   node` (Ready condition message), SSH to the node, `systemctl status
   containerd`, `journalctl -u kubelet`, `crictl ps` fails.
3. `... revert` and confirm orphaned containers are cleaned up
   (`crictl ps` matches `kubectl get pods --field-selector spec.nodeName=worker-1`).

## Results

| Measure | Value |
| --- | --- |
| Time to NotReady | |
| Time to alert | |
| Time to diagnose | |
| Probe failures | |
| Orphans after revert | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
