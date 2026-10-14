#!/usr/bin/env bash
# Package apps/gateway-api/migrations into a ConfigMap and run the migrate Job.
# Exits non-zero if the Job fails; prints the Job logs either way.
set -euo pipefail

NS="satark-data"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_ID="$(date +%Y%m%d%H%M%S)"
JOB="satark-migrate-${RUN_ID}"

kubectl -n "${NS}" create configmap satark-migrations \
  --from-file="${ROOT}/apps/gateway-api/migrations" \
  --dry-run=client -o yaml | kubectl apply -f -

sed "s/^  name: satark-migrate$/  name: ${JOB}/" \
  "${ROOT}/gitops/platform/data/migrations/job.yaml" | kubectl apply -f -

status=0
kubectl -n "${NS}" wait --for=condition=Complete "job/${JOB}" --timeout=10m || status=$?
kubectl -n "${NS}" logs "job/${JOB}" --all-containers || true
exit "${status}"
