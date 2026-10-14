# satark-common

Library chart shared by every Satark service chart. It renders nothing on its
own; a service chart declares it as a dependency and includes the templates.

```yaml
# charts/<svc>/Chart.yaml
dependencies:
  - name: satark-common
    version: 0.1.0
    repository: file://../satark-common
```

```yaml
# charts/<svc>/templates/deployment.yaml
{{ include "satark-common.deployment" . }}
```

## Values contract

| Key | Meaning |
|---|---|
| `image.repository`, `image.tag`, `image.digest` | Image, e.g. `ghcr.io/guru-bharadwaj20/satark/<svc>`; a digest wins over the tag |
| `replicaCount` | Replicas when autoscaling is off |
| `service.port` | Service and container port |
| `env` | Map `NAME: value` or a list of `EnvVar`; `extraEnv` / `envFrom` for secrets |
| `resources` | Requests and limits |
| `livenessProbe`, `readinessProbe`, `startupProbe` | Container probes |
| `serviceAccount.create`, `serviceAccount.name` | Service account (token not mounted) |
