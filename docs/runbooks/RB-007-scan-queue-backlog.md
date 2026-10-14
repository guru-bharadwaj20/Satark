# RB-007: Scan queue backlog

| | |
|---|---|
| **Alert** | `LinkScanQueueBacklog` (warning), `LinkScanQueueStalled` (critical) |
| **Service** | NATS `SCAN_JOBS` stream → `linkscan-operator` intake → sandbox pods |
| **User impact** | Telegram replies are not updated with the sandbox result, or the update is late. The fast verdict still works. |
| **Related** | ADR-006, RB-008, `apps/linkscan-operator/config/keda/scaledobject.yaml` |

## Symptoms

- `nats_consumer_num_pending{consumer_name="linkscan-operator"}` keeps growing.
- Users get the first reply but no "Sandbox check" edit.
- `nats stream info SCAN_JOBS` shows many messages, or `discard: new` rejections
  in url-intel logs (`nats: maximum messages exceeded`).

## Triage (5 minutes)

```bash
# 1. Queue depth and consumer state
kubectl -n satark-data exec deploy/nats-box -- nats consumer info SCAN_JOBS linkscan-operator
#   "Unprocessed Messages" = backlog; "Outstanding Acks" = jobs being admitted

# 2. Is KEDA scaling the operator?
kubectl -n satark get scaledobject linkscan-operator-controller-manager
kubectl -n satark get hpa,deploy -l app.kubernetes.io/name=linkscan-operator

# 3. Are scans running, or stuck before they start?
kubectl -n satark-sandbox get linkscans --sort-by=.metadata.creationTimestamp | tail -20
kubectl -n satark-sandbox get pods -l satark.io/role=sandbox -o wide
```

## Decision tree

1. **Replicas at `maxReplicaCount` (6), pods Running and finishing:** this is
   real load (for example a scam campaign link going viral).
   - Raise `maxReplicaCount` or `--max-active-scans`, but only if sandbox nodes
     have spare CPU and memory (`kubectl top nodes -l satark.io/gvisor=true`).
   - Duplicate links can be served from the url-intel cache. Check that the cache
     is being hit.
2. **KEDA not scaling (`READY=False`, or the HPA has no metrics):**
   - `kubectl -n keda logs deploy/keda-operator | grep -i jetstream`
   - Check the monitoring endpoint: `kubectl -n satark-data port-forward svc/nats 8222` then
     `curl localhost:8222/jsz?consumers=true`.
3. **Operator replicas up but no new LinkScans being created:**
   - `kubectl -n satark logs deploy/linkscan-operator-controller-manager | grep intake`
   - Readiness check `nats` failing means the operator lost its NATS connection.
   - RBAC errors (`forbidden ... linkscans`) mean the ClusterRole is out of date. Re-sync the app.
4. **LinkScans created but stuck Pending:** sandbox pods cannot start, so the
   intake's capacity fills with stuck scans. Go to **RB-008**.
5. **Consumer missing (`consumer not found`):** re-run the Argo CD PostSync job:
   `kubectl -n satark-data delete job nats-streams` and sync the `nats` app.

## Mitigation

- Short-term relief: scale manually with
  `kubectl -n satark scale deploy linkscan-operator-controller-manager --replicas=N`
  (KEDA takes over again later).
- Jobs older than 10 minutes expire from the stream (`max_age`). Telegram users
  would no longer be waiting for those, so do not replay them.
- If the queue is flooded with one abusive domain, block it in url-intel so
  those links are not queued at all.

## Verify recovery

- `nats_consumer_num_pending` is back to around 0 and the alert is resolved.
- New Telegram link checks get the sandbox edit within about 30 s.

## Follow-up

- Record peak queue depth and duration in the incident notes.
- If the backlog was organic, revisit the sandbox node count and KEDA limits.
