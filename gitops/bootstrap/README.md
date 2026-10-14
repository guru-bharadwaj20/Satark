# gitops/bootstrap

App-of-apps root for Argo CD. `root.yaml` is applied once by the
`argocd_bootstrap` Ansible role; from then on Argo CD syncs every manifest in
this folder, including `root.yaml` itself.

| File | Purpose |
|---|---|
| `root.yaml` | The root Application (manages this folder) |
| `projects.yaml` | `platform` and `satark` AppProjects |
| `services-appset.yaml` | ApplicationSet: one Application per service per environment |
