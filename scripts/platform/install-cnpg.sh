#!/usr/bin/env bash
# Install (or upgrade) the CloudNativePG operator into the cnpg-system namespace.
# Usage: scripts/platform/install-cnpg.sh   (uses the current kubectl context)
set -euo pipefail

CHART_VERSION="${CNPG_CHART_VERSION:-0.22.1}" # operator 1.24.x
NAMESPACE="cnpg-system"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null 2>&1 || true
helm repo update cnpg >/dev/null

helm upgrade --install cnpg cnpg/cloudnative-pg \
  --namespace "${NAMESPACE}" --create-namespace \
  --version "${CHART_VERSION}" \
  --values "${ROOT}/gitops/platform/core/cloudnative-pg/values.yaml" \
  --wait --timeout 5m

kubectl -n "${NAMESPACE}" rollout status deployment/cnpg-cloudnative-pg --timeout=180s
kubectl wait --for=condition=Established crd/clusters.postgresql.cnpg.io --timeout=60s
echo "CloudNativePG operator ready (chart ${CHART_VERSION})."
