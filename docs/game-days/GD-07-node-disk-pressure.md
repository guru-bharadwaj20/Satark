# GD-07: Disk pressure on a node

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
| Script | `tests/chaos/gd-07-disk-pressure.sh` |
| Expected duration | 40 minutes |

## Hypothesis

When free space on `worker-2`'s container filesystem drops below 10%:

- `NodeFilesystemAlmostOutOfSpace` (node-exporter) fires first, at 15% free.
- The kubelet sets `DiskPressure=True` and taints the node
  `node.kubernetes.io/disk-pressure:NoSchedule`.
- The kubelet first garbage-collects unused images, then evicts pods,
  BestEffort first, then Burstable pods exceeding their ephemeral-storage
  requests. Guaranteed pods are evicted last.
- Evicted pods are recreated elsewhere; evicted objects remain as `Failed`
  with reason `Evicted` until cleaned up.
- `sandbox-browser` (large image, writes screenshots to emptyDir) is the most
  likely to be evicted.

## Blast radius and abort criteria

- Scope: one worker. Abort if free space drops below 3% (risk to etcd or
  the kubelet itself) or the CNPG instance on the node is evicted.
- Abort action: `... revert`.

## Procedure

1. `... observe`, then `... inject`.
2. Responder starts from the filesystem alert. Expected path: `kubectl
   describe node` (DiskPressure, taint), `kubectl get pods -A | grep Evicted`,
   SSH and `du -xh --max-depth=2 /var/lib | sort -h`, find the filler.
3. `... revert`.

## Things to check

- Do all pods set `ephemeral-storage` requests and limits?
- Is container log rotation (`containerLogMaxSize`) configured?
- Did image GC free enough to matter? How big are our images?

## Results

| Measure | Value |
| --- | --- |
| Time to alert | |
| Time to DiskPressure | |
| Pods evicted (by QoS class) | |
| Time to diagnose | |
| Probe failures | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
