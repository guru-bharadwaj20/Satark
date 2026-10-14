# RB-008: Link-scan sandbox failures

| | |
|---|---|
| **Alert** | `LinkScanFailureRateHigh`, `LinkScanPodCreateErrors` |
| **Service** | `linkscan-operator`, sandbox pods (`sandbox-browser` under gVisor) in `satark-sandbox` |
| **User impact** | Link checks show "the sandbox could not open this link". The fast checks (redirects, domain age, feeds) still work. |
| **Related** | ADR-006, RB-007, RB-001 (CrashLoop/ImagePull) |

## Symptoms

- `satark_linkscan_scans_total{phase=~"Failed|TimedOut"}` rising.
- `kubectl get linkscans -n satark-sandbox` shows many `Failed` / `TimedOut`.
- Warning events on LinkScans: `PodFailed`, `InvalidResult`, `DeadlineExceeded`,
  `PodCreateFailed`, `PodLost`, `ResultPublishFailed`.

## Triage

```bash
kubectl -n satark-sandbox get linkscans -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,MSG:.status.message | tail -20
kubectl -n satark-sandbox get events --field-selector type=Warning --sort-by=.lastTimestamp | tail -20
kubectl -n satark-sandbox describe linkscan <name>
kubectl -n satark-sandbox logs <name>-sandbox          # sandbox output (kept until TTL)
```

Group the failures by reason, then follow the matching section.

### A. `PodCreateFailed` (no pods at all)

| Message | Cause | Fix |
|---|---|---|
| `RuntimeClass "gvisor" not found` | RuntimeClass not synced | Sync `gitops/platform/sandbox` |
| `violates PodSecurity` / Kyverno denial | A policy change rejects the pod spec | Compare the policy with `internal/controller/pod.go`. Do not weaken the pod. Add a scoped exception if one is truly needed. |
| `exceeded quota` | ResourceQuota in `satark-sandbox` | Raise the quota or lower `--max-active-scans` |
| `forbidden: ... pods` | Operator RBAC | Re-sync the operator, then check `config/rbac/role.yaml` |

### B. Pods stuck `Pending` → scans end `TimedOut` with "did not finish within"

- `kubectl describe pod`: `0/N nodes are available: node(s) didn't match Pod's node affinity`
  means no node has `satark.io/gvisor=true`. Label nodes that have run the `gvisor` role.
- `ErrImagePull` / `ImagePullBackOff`: see RB-001. Check that the `--sandbox-image` tag exists.
- `failed to create containerd task: ... runsc`: runsc is broken on that node.
  Run `ansible-playbook ... --tags gvisor --limit <node>` and check
  `journalctl -u containerd` and `/var/log/runsc/`.

### C. `DeadlineExceeded` (pod ran but was killed)

- Some slow sites are expected. Worry when the rate is high across many domains.
- Check sandbox node CPU (`kubectl top nodes -l satark.io/gvisor=true`). gVisor
  is CPU-heavy, so a starved node causes mass timeouts. Lower `--max-active-scans`
  or add nodes.
- Check DNS. If the public resolvers (1.1.1.1, 9.9.9.9) cannot be reached from
  sandbox nodes, every navigation hangs. Test with the debug pod below.

### D. `PodFailed` / `InvalidResult`

- `sandbox error: net::ERR_NAME_NOT_RESOLVED` / `ERR_CONNECTION_REFUSED` for
  one URL: the site is down. This is not an incident.
- The same error for every URL means egress is broken. Check that no new
  NetworkPolicy removed the public-egress rule, and use Hubble:
  `hubble observe -n satark-sandbox --verdict DROPPED --last 50`.
- `crash: ...` or Chromium `Target closed`: usually OOM. Look for
  `OOMKilled` in `kubectl describe pod`. Check `/dev/shm` and memory limits in `pod.go`.
- `decode sandbox result`: the image and the operator disagree on the result
  format. Roll back the newer of the two.

### E. `ResultPublishFailed`

- The operator cannot publish to `scan.results.*`. Check NATS health and that
  the `SCAN_RESULTS` stream exists (`nats stream info SCAN_RESULTS`). Scans are
  kept (TTL is paused) until publishing works, so nothing is lost.

## Debug pod (same isolation as a scan)

```bash
kubectl apply -f - <<'EOF'
apiVersion: scan.satark.io/v1alpha1
kind: LinkScan
metadata: {name: debug-example, namespace: satark-sandbox}
spec: {url: "https://example.com/", timeoutSeconds: 60, ttlSecondsAfterFinished: 600}
EOF
kubectl -n satark-sandbox get linkscan debug-example -w
```

If `example.com` fails too, the problem is in the platform (runtime, network or
nodes), not in the target sites.

## Rules during an incident

- **Do not** turn off gVisor, add `privileged`, remove the NetworkPolicy, or
  mount extra credentials to "get scans working". Users are better served by
  "not checked" than by an exposed cluster (ADR-006).
- Do not copy user URLs into tickets or chats. Refer to them by LinkScan name.

## Verify recovery

- The failure rate alert is resolved, and `debug-example` reaches `Succeeded`.
- New Telegram checks show a "Sandbox check" section.
