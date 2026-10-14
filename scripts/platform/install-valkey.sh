#!/usr/bin/env bash
# Create the Valkey password (once) and deploy Valkey into satark-data.
# The same password is copied into the app namespaces (satark-dev, satark).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

kubectl apply -f "${ROOT}/gitops/platform/data/namespace.yaml"

if ! kubectl -n satark-data get secret valkey-auth >/dev/null 2>&1; then
  PASSWORD="$(openssl rand -hex 24)"
  for ns in satark-data satark-dev satark; do
    kubectl create namespace "${ns}" --dry-run=client -o yaml | kubectl apply -f -
    kubectl -n "${ns}" create secret generic valkey-auth --from-literal=password="${PASSWORD}"
  done
  unset PASSWORD
fi

kubectl apply -f "${ROOT}/gitops/platform/data/valkey/"
kubectl -n satark-data rollout status statefulset/valkey --timeout=180s
kubectl -n satark-data exec valkey-0 -- valkey-cli ping
