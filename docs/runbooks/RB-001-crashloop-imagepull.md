# RB-001: Pod in CrashLoopBackOff or ImagePullBackOff

| | |
|---|---|
| **Alerts** | `SatarkPodCrashLooping`, `SatarkImagePullFailing`, `SatarkFrequentRestarts`, `SatarkTargetDown` |
| **Severity** | Critical (crash loop / image pull), warning (restarts, target down) |
| **Impact** | The affected service runs with fewer replicas or not at all. Scans that need it fail or fall back to partial verdicts. |
| **Dashboards** | Satark / Cluster health (Workloads row), Satark / Scan pipeline |

## 1. Triage (2 minutes)

```bash
NS=satark                       # from the alert's namespace label
POD=<pod from the alert>
kubectl -n $NS get pod $POD -o wide
kubectl -n $NS describe pod $POD | sed -n '/Containers:/,/Events:/p;/Events:/,$p'
```

Read `State`, `Last State`, `Reason`, `Exit Code` and the Events at the bottom.
Then decide which branch below applies.

Check whether the change is recent:

```bash
kubectl -n argocd get applications -o wide | grep -v Synced  # anything mid-sync or degraded?
kubectl -n $NS rollout history deploy/<service>
```

If a rollout in the last hour is the cause and the fix is not obvious,
**roll back first, debug second** (section 4).

## 2. CrashLoopBackOff

| Exit code / reason | Likely cause | Next step |
|---|---|---|
| `OOMKilled` / 137 | Memory limit too low or a leak | Check memory in the Scan pipeline dashboard; raise the limit in the service's Helm values via Git |
| 1 / 2 with an error in the logs | Bad config, missing env var or Secret, failed migration | Read the logs (below) |
| 0 | Process exits normally; wrong command/args | Compare the image's entrypoint with the chart |
| `Error` with liveness probe failures in Events | Probe too strict, or `/healthz` blocked on a dependency | `/healthz` must not check dependencies; only `/readyz` does |

```bash
kubectl -n $NS logs $POD -c <container> --previous --tail=100
```

or in Grafana Explore (Loki):

```logql
{namespace="satark", pod="<pod>"} | json | level=~"ERROR|WARN"
```

Common Satark causes:

- **Postgres/Valkey/NATS unreachable at start-up.** Check `kubectl -n satark-data get pods`.
  If a data service is down, fix that first; the app will recover on its own.
- **Missing Secret key** (e.g. LLM API key, Telegram token): Events show
  `CreateContainerConfigError`. Check that the ExternalSecret / Secret exists
  and has the expected key: `kubectl -n $NS get secret <name> -o jsonpath='{.data}' | jq 'keys'`.
- **NetworkPolicy blocks a dependency** after a policy change: look for drops
  in Hubble: `hubble observe -n $NS --verdict DROPPED --last 50`.

## 3. ImagePullBackOff / ErrImagePull

```bash
kubectl -n $NS get pod $POD -o jsonpath='{.spec.containers[*].image}{"\n"}'
kubectl -n $NS describe pod $POD | grep -A3 -i 'failed to pull'
```

| Message | Fix |
|---|---|
| `not found` / `manifest unknown` | The tag was never pushed. Check the CI run for that commit in GitHub Actions and the GHCR package. Revert the image tag in Git if CI failed. |
| `unauthorized` / `denied` | GHCR pull secret missing or expired: check `imagePullSecrets` and the `ghcr-pull` Secret in the namespace. |
| `i/o timeout` / `no such host` | Node cannot reach ghcr.io: see RB-005 (DNS) and check node egress. |
| Kyverno `image signature` failure | Image not signed by the release workflow. Do **not** disable the policy; rebuild through CI. |

## 4. Mitigate

Roll back through Git, not with `kubectl` (Argo CD would revert manual changes):

```bash
git revert <commit that bumped the image or values>
git push   # Argo CD syncs the previous version
```

If you must act before CI and Argo CD can (critical outage), pause auto-sync
for that app, roll back, and record it in the incident notes:

```bash
argocd app set <app> --sync-policy none
kubectl -n $NS rollout undo deploy/<service>
# after the Git revert has landed:
argocd app set <app> --sync-policy automated --self-heal --auto-prune
```

## 5. Verify

- `kubectl -n $NS get pods` shows the pod `Running` and `READY` for 10 minutes.
- The alert resolves in the Telegram ops channel.
- Scan pipeline dashboard: error ratio back to baseline.

## 6. Follow-up

Open an issue with the root cause. If a probe, limit or missing alert made this
worse, fix it in the chart. If an incident was declared, write the review.
