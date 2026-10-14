# gitops/bootstrap

App-of-apps root for Argo CD. `root.yaml` is applied once by the
`argocd_bootstrap` Ansible role; from then on Argo CD syncs every manifest in
this folder, including `root.yaml` itself.

| File | Purpose |
|---|---|
| `root.yaml` | The root Application (manages this folder) |
| `projects.yaml` | `platform` and `satark` AppProjects |
| `services-appset.yaml` | ApplicationSet: one Application per service per environment |
| `platform-crds.yaml` | Wave 0: CRDs from `gitops/platform/crds` |
| `platform-core.yaml` | Wave 10: platform components from `gitops/platform/core` |
| `platform-data.yaml` | Wave 20: Postgres, Valkey and NATS from `gitops/platform/data` |

## Sync waves

| Wave | What |
|---|---|
| -10 | AppProjects |
| -5 | Argo CD itself |
| 0 | CRDs |
| 10 | Platform |
| 20 | Data |
| 30 | Application services (ApplicationSet) |

Waves between child Applications only block correctly because Argo CD is
configured with a health check for `argoproj.io/Application`
(see `gitops/platform/argocd`).

`argocd.yaml` (wave -5) points Argo CD at `gitops/platform/argocd`, so
upgrades and config changes to Argo CD itself go through Git. Auto-prune is
disabled on it on purpose.
