# gitops/platform/crds (sync wave 0)

CustomResourceDefinitions (operator CRDs, Gateway API, monitoring).

Put Argo CD `Application` manifests (or plain manifests) for this layer here.
Inside the layer, order resources further with
`argocd.argoproj.io/sync-wave` annotations.
