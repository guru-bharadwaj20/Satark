# gitops/platform/core (sync wave 10)

Platform components (cert-manager, ingress, observability, policy).

Put Argo CD `Application` manifests (or plain manifests) for this layer here.
Inside the layer, order resources further with
`argocd.argoproj.io/sync-wave` annotations.
