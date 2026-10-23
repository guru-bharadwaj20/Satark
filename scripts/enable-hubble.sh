#!/usr/bin/env bash
# Enable Hubble Relay + UI + metrics on the existing Cilium release.
# Run after kube-prometheus-stack is installed (ServiceMonitor CRD must exist).
set -euo pipefail

CILIUM_VERSION="${CILIUM_VERSION:-1.16.3}"
VALUES="$(dirname "$0")/../gitops/platform/hubble/hubble-values.yaml"

helm repo add cilium https://helm.cilium.io >/dev/null 2>&1 || true
helm repo update cilium >/dev/null

helm upgrade cilium cilium/cilium \
  --namespace kube-system \
  --version "${CILIUM_VERSION}" \
  --reuse-values \
  -f "${VALUES}" \
  --wait --timeout 10m

kubectl -n kube-system rollout status deploy/hubble-relay --timeout=5m
kubectl -n kube-system rollout status deploy/hubble-ui --timeout=5m

echo "Hubble UI ready: kubectl -n kube-system port-forward svc/hubble-ui 12000:80"
echo "Then open http://localhost:12000"
