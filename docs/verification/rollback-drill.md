# Rollback drill

A bad release of gateway-api in dev was rolled back with `git revert`, timed
by `scripts/rollback-drill.sh`. This is the procedure in SOP-04.

| Field | Value |
|---|---|
| Date | 2026-10-17 |
| Operator | guru-bharadwaj20 |
| Cluster | on-prem lab, kubeadm v1.35.9, MVP from the MVP deploy |
| Argo CD | v3.5.3, installed from `gitops/platform/argocd` (annotation tracking, 180 s reconcile) |
| Application | `gateway-api-dev`: same spec the `satark-services` ApplicationSet generates |
| Bad release | `4b08839` sets dev tag `0.1.1` (never built, so `ErrImagePull`) |
| Rollback | `a5bde3b` = `git revert 4b08839`, back to tag `0.1.0` |

## Result

```
05:54:25 reverting 4b08839: gateway-api (dev) 0.1.1 -> 0.1.0
05:54:28 revert pushed after 3s
05:54:32 Argo CD applied 0.1.0 after 4s
05:54:33 rollout complete: total 8s (push 3s, sync 4s, rollout 1s)
```

The timing is recorded in `docs/game-days/rollback-timings.csv`.

| Stage | Seconds |
|---|---|
| `git revert` + push | 3 |
| Argo CD fetch and apply (refresh requested by the script) | 4 |
| Rollout to Ready | 1 |
| **Total** | **8** |

- **Why the rollout took 1 s.** The bad pod never started, so the old
  ReplicaSet kept serving (`maxSurge` 1, `maxUnavailable` 0). The revert only
  scaled the failed ReplicaSet away. A bad image that *does* start would add
  the pod startup time, about 5–10 s for gateway-api.
- **The 4 s sync relies on a refresh.** The script annotates the Application
  to ask for one. Without it, Argo CD notices the revert on its next poll,
  which can take up to `timeout.reconciliation` (180 s). A GitHub webhook to
  Argo CD would remove that wait.
- **No downtime during the bad release.** The old pod stayed Ready. Afterwards
  Argo CD showed `Synced/Healthy`, and `scripts/smoke-test.sh` passed again.

## Setup

Run from Git Bash on the Windows host with
`KUBECONFIG=~/.kube/satark-onprem.conf`:

```
kubectl create namespace argocd
kubectl apply --server-side --force-conflicts -k gitops/platform/argocd
kubectl apply -f gitops/bootstrap/projects.yaml
kubectl apply -f <gateway-api-dev Application>   # ApplicationSet template rendered for gateway-api/dev
IMAGE_TAG=0.1.0 NODES="worker-1 worker-2" scripts/load-lab-images.sh gateway-api
```

The full app-of-apps (`gitops/bootstrap/root.yaml`) was not applied. It
would deploy every platform component and all 11 services in dev and prod.
Most of those need things the lab doesn't have yet: sealed secrets, AWS,
cert-manager, and more memory than the 4 GB workers.

The public Gateway `satark-public` was applied with a 30-day self-signed
`*.satark.example` certificate in Secret `envoy-gateway-system/satark-public-tls`.
It stays in the cluster only; cert-manager replaces it later. It got
`192.168.56.240` from MetalLB. A scan through
`https://api.satark.example/v1/scan/text` returned 200 with rate-limit
headers, and plain HTTP returned 301 to HTTPS.

## Issues found and fixes

- Argo CD v2.13.3 could not compare any Deployment on Kubernetes 1.35:
  `.status.terminatingReplicas: field not declared in schema`. Both pins
  (`gitops/platform/argocd/kustomization.yaml` and the `argocd_bootstrap`
  role) are now v3.5.3.
- `gitops/platform/argocd` did not build. The ConfigMap patches carried
  `namespace: argocd`, but the upstream manifest has no namespace when the
  patches are applied. The patches now leave the namespace to the kustomization.
- The HTTPRoutes were permanently OutOfSync. Argo CD's server-side diff does
  not apply CRD defaults, so the API-added `group`, `kind` and `weight` always
  showed up as a difference. The `satark-common` template now sets them.
