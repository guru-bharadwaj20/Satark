#!/usr/bin/env bash
# Install the Gateway API CRDs and Envoy Gateway, then the "satark" GatewayClass.
set -euo pipefail

EG_VERSION="${ENVOY_GATEWAY_VERSION:-v1.2.1}"
NAMESPACE="envoy-gateway-system"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

kubectl apply --server-side -k "${ROOT}/gitops/platform/crds/gateway-api"
kubectl wait --for=condition=Established crd/gateways.gateway.networking.k8s.io --timeout=60s

helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
  --version "${EG_VERSION}" \
  --namespace "${NAMESPACE}" --create-namespace \
  --values "${ROOT}/gitops/platform/core/envoy-gateway/values.yaml" \
  --wait --timeout 5m

kubectl -n "${NAMESPACE}" rollout status deployment/envoy-gateway --timeout=180s
kubectl apply -f "${ROOT}/gitops/platform/core/envoy-gateway/gatewayclass.yaml"
kubectl wait --for=condition=Accepted gatewayclass/satark --timeout=120s
echo "Envoy Gateway ${EG_VERSION} ready; GatewayClass satark accepted."
