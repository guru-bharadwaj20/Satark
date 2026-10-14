#!/usr/bin/env bash
# Run a k6 profile and keep the summary next to a snapshot of pod resource use,
# so a failed threshold can be traced to the component that saturated first.
#
#   BASE_URL=https://api.satark.example tests/load/run.sh stress
set -euo pipefail
PROFILE="${1:-smoke}"
DIR="$(cd "$(dirname "$0")" && pwd)"
OUT="$DIR/results/$PROFILE-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$OUT"

snapshot() {
  kubectl top pods -n satark --containers > "$OUT/top-$1.txt" 2>/dev/null || true
  kubectl get hpa,scaledobjects -n satark > "$OUT/autoscale-$1.txt" 2>/dev/null || true
}

snapshot before
( while sleep 30; do snapshot "$(date -u +%H%M%S)"; done ) &
SNAP=$!
trap 'kill $SNAP 2>/dev/null || true' EXIT

k6 run -e BASE_URL="${BASE_URL:?}" -e PROFILE="$PROFILE" \
  --summary-export "$OUT/summary.json" "$DIR/scenarios.js" | tee "$OUT/k6.txt"
echo "Results in $OUT"
