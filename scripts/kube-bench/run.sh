#!/usr/bin/env bash
# Run the kube-bench CIS benchmark Job and save its JSON report (task 9.8).
# Usage: scripts/kube-bench/run.sh [output-dir]   (default docs/compliance/kube-bench)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="${1:-$ROOT/docs/compliance/kube-bench}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="$OUT_DIR/kube-bench-$STAMP.json"

mkdir -p "$OUT_DIR"
kubectl delete job kube-bench -n kube-bench --ignore-not-found --wait=true
kubectl apply -f "$ROOT/scripts/kube-bench/job.yaml"
kubectl wait --for=condition=complete job/kube-bench -n kube-bench --timeout=300s
kubectl logs -n kube-bench job/kube-bench > "$OUT"

if command -v jq >/dev/null; then
  jq -r '.Totals | "PASS=\(.total_pass) FAIL=\(.total_fail) WARN=\(.total_warn) INFO=\(.total_info)"' "$OUT"
fi
echo "Report written to $OUT"
