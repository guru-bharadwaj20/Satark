#!/usr/bin/env bash
# Smoke tests for a Satark MVP deployment.
#   - every Deployment is fully available
#   - gateway-api /healthz and /readyz return 200
#   - a known scam-pattern text gets a non-"no_red_flags" verdict
#   - rate-limit headers are present
#   - optionally the same scan through the public Gateway (GATEWAY_URL)
# Usage: NAMESPACE=satark-dev [GATEWAY_URL=https://api.example] scripts/smoke-test.sh
set -euo pipefail

NAMESPACE="${NAMESPACE:-satark-dev}"
LOCAL_PORT="${LOCAL_PORT:-18080}"
fail=0
pass() { printf 'PASS  %s\n' "$1"; }
bad() { printf 'FAIL  %s\n' "$1"; fail=1; }

for d in text-analyzer qr-decoder gateway-api telegram-bot; do
  if kubectl -n "${NAMESPACE}" rollout status "deploy/${d}" --timeout=120s >/dev/null 2>&1; then
    pass "deployment ${d} available"
  else
    bad "deployment ${d} not available"
  fi
done

kubectl -n "${NAMESPACE}" port-forward svc/gateway-api "${LOCAL_PORT}:8080" >/dev/null 2>&1 &
PF_PID=$!
trap 'kill ${PF_PID} 2>/dev/null || true' EXIT
for _ in $(seq 1 20); do
  curl -fsS "http://127.0.0.1:${LOCAL_PORT}/healthz" >/dev/null 2>&1 && break
  sleep 0.5
done
BASE="http://127.0.0.1:${LOCAL_PORT}"

for p in /healthz /readyz; do
  code="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}${p}")"
  [[ "${code}" == 200 ]] && pass "GET ${p} -> 200" || bad "GET ${p} -> ${code}"
done

# Masked, synthetic sample (no real personal data).
BODY='{"text":"Dear Customer, your electricity connection will be disconnected tonight at 9:30 PM. Contact our officer 98XXXXXX21 immediately.","lang":"en"}'
scan() {
  curl -sS -D "$2" -o "$3" -w '%{http_code}' -H 'Content-Type: application/json' \
    -X POST --data "${BODY}" "$1/v1/scan/text"
}
check_scan() {
  local base="$1" label="$2" hdr body code
  hdr="$(mktemp)"; body="$(mktemp)"
  code="$(scan "${base}" "${hdr}" "${body}")"
  if [[ "${code}" != 200 ]]; then
    bad "${label} scan -> HTTP ${code}"
  elif grep -q '"verdict"' "${body}" && ! grep -q '"no_red_flags"' "${body}"; then
    pass "${label} scan flags the scam sample"
  else
    bad "${label} scan verdict unexpected: $(head -c 300 "${body}")"
  fi
  grep -qi '^x-ratelimit-limit:' "${hdr}" && pass "${label} rate-limit headers" \
    || bad "${label} missing X-RateLimit-Limit"
  rm -f "${hdr}" "${body}"
}

check_scan "${BASE}" "in-cluster"
[[ -n "${GATEWAY_URL:-}" ]] && check_scan "${GATEWAY_URL%/}" "gateway"

if [[ "${fail}" -ne 0 ]]; then
  echo "Smoke tests FAILED"
  exit 1
fi
echo "Smoke tests passed"
