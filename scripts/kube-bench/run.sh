#!/usr/bin/env bash
# Run the kube-bench CIS benchmark Jobs and save their JSON reports (task 9.8).
# Writes kube-bench-<stamp>-control-plane.json and kube-bench-<stamp>-worker.json.
# Usage: scripts/kube-bench/run.sh [output-dir]   (default docs/compliance/kube-bench)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="${1:-$ROOT/docs/compliance/kube-bench}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
JOBS=(kube-bench:control-plane kube-bench-worker:worker)

mkdir -p "$OUT_DIR"
for j in "${JOBS[@]}"; do
  kubectl delete job "${j%%:*}" -n kube-bench --ignore-not-found --wait=true
done
kubectl apply -f "$ROOT/scripts/kube-bench/job.yaml"

for j in "${JOBS[@]}"; do
  job="${j%%:*}" role="${j#*:}"
  out="$OUT_DIR/kube-bench-$STAMP-$role.json"
  kubectl wait --for=condition=complete "job/$job" -n kube-bench --timeout=300s
  # kube-bench prints warnings on the same stream; keep only the JSON line.
  kubectl logs -n kube-bench "job/$job" | grep '^{' > "$out"
  node=$(kubectl get pods -n kube-bench -l "job-name=$job" -o jsonpath='{.items[0].spec.nodeName}')
  python3 - "$out" "$node" <<'EOF'
import json, sys
t = json.load(open(sys.argv[1]))["Totals"]
print(f"{sys.argv[2]}: PASS={t['total_pass']} FAIL={t['total_fail']} "
      f"WARN={t['total_warn']} INFO={t['total_info']}  ({sys.argv[1]})")
EOF
done
