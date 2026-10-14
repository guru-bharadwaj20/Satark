#!/usr/bin/env bash
# Runs the N-1 -> N upgrade while k6 drives constant traffic, then prints the
# error count and node versions for the record.
#
#   tests/load/upgrade-under-load.sh 1.31 inventory/prod
set -euo pipefail

TARGET_MINOR="${1:?target minor, e.g. 1.31}"
INVENTORY="${2:-inventory/prod}"
BASE_URL="${BASE_URL:?set BASE_URL to the public API}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${OUT:-$ROOT/tests/load/results/upgrade-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "$OUT"

kubectl get nodes -o wide > "$OUT/nodes-before.txt"

k6 run -e BASE_URL="$BASE_URL" -e DURATION="${DURATION:-120m}" \
  --summary-export "$OUT/k6-summary.json" \
  "$ROOT/tests/load/upgrade-soak.js" > "$OUT/k6.log" 2>&1 &
K6_PID=$!
trap 'kill $K6_PID 2>/dev/null || true' EXIT

sleep 120   # baseline before the first node is touched
start=$(date -u +%s)
( cd "$ROOT/infra/ansible" && \
  ansible-playbook -i "$INVENTORY" playbooks/upgrade.yml \
    -e k8s_upgrade_target_minor="$TARGET_MINOR" ) | tee "$OUT/ansible.log"
end=$(date -u +%s)
sleep 120   # tail after the last node
kill -INT $K6_PID; wait $K6_PID || true
trap - EXIT

kubectl get nodes -o wide > "$OUT/nodes-after.txt"

errors=$(jq -r '.metrics.satark_upgrade_errors.count // 0' "$OUT/k6-summary.json")
reqs=$(jq -r '.metrics.http_reqs.count' "$OUT/k6-summary.json")
p95=$(jq -r '.metrics.http_req_duration["p(95)"]' "$OUT/k6-summary.json")
cat <<EOF | tee "$OUT/summary.txt"
target_minor=$TARGET_MINOR
upgrade_seconds=$(( end - start ))
requests=$reqs
errors=$errors
p95_ms=$p95
EOF
