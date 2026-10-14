# gitops/platform/data (sync wave 20)

Data layer (CloudNativePG cluster satark-pg, Valkey, NATS).

Put Argo CD `Application` manifests (or plain manifests) for this layer here.
Inside the layer, order resources further with
`argocd.argoproj.io/sync-wave` annotations.
