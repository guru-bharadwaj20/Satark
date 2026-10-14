#!/usr/bin/env bash
# Create the application credentials for satark-pg if they don't exist yet,
# then apply the Cluster. The password is generated locally and never printed.
set -euo pipefail

NS="satark-data"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

kubectl apply -f "${ROOT}/gitops/platform/data/namespace.yaml"

if ! kubectl -n "${NS}" get secret satark-pg-app >/dev/null 2>&1; then
  PASSWORD="$(openssl rand -base64 32 | tr -d '/+=' | cut -c1-32)"
  kubectl -n "${NS}" create secret generic satark-pg-app \
    --type=kubernetes.io/basic-auth \
    --from-literal=username=satark \
    --from-literal=password="${PASSWORD}"
  unset PASSWORD
fi

kubectl apply -f "${ROOT}/gitops/platform/data/postgres/"
kubectl -n "${NS}" wait --for=condition=Ready cluster/satark-pg --timeout=10m
kubectl -n "${NS}" get pods -l cnpg.io/cluster=satark-pg -o wide
