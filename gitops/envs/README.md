# gitops/envs

One folder per environment. The `satark-services` ApplicationSet reads:

- `<env>/env.json`: environment name, target namespace and Git revision.
- `<env>/apps/<service>/values.yaml`: Helm overrides for `charts/<service>`.

| Env | Namespace | Replicas | Image tag updates |
|---|---|---|---|
| dev | `satark-dev` | 1 | Automatic, committed by CI on every merge to `main` |
| prod | `satark` | 2 for stateless APIs | Promotion PR with required review (SOP-04) |

To add a service to an environment, add `apps/<service>/values.yaml`.
To remove it, delete the folder; Argo CD prunes the Application.
