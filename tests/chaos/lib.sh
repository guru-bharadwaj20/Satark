#!/usr/bin/env bash
# Shared helpers for game-day scripts. Source this file; do not run it.
#
# Each game-day script accepts: inject | observe | revert
# and appends timestamped events to $GD_LOG so the write-up can be built from it.

set -euo pipefail

GD_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GD_NAME="${GD_NAME:-$(basename "$0" .sh)}"
GD_LOG="${GD_LOG:-$GD_ROOT/docs/game-days/logs/${GD_NAME}-$(date -u +%Y%m%d).log}"
mkdir -p "$(dirname "$GD_LOG")"

event() {
  local line
  line="$(date -u +%Y-%m-%dT%H:%M:%SZ) [$GD_NAME] $*"
  echo "$line" | tee -a "$GD_LOG"
}

# Refuse to run against anything that is not the expected cluster.
require_context() {
  local want="${GD_CONTEXT:-satark-prod}"
  local have
  have="$(kubectl config current-context)"
  if [[ "$have" != "$want" ]]; then
    echo "Current context is '$have', expected '$want'. Set GD_CONTEXT to override." >&2
    exit 1
  fi
}

confirm() {
  [[ "${CONFIRM:-}" == "yes" ]] && return 0
  read -r -p "$1 Type 'yes' to continue: " ans
  [[ "$ans" == "yes" ]] || { echo "aborted"; exit 1; }
}

# Snapshot cluster health into the log (uses the kubectl-satark plugin when present).
snapshot() {
  event "snapshot: $1"
  if kubectl satark version >/dev/null 2>&1; then
    kubectl satark doctor --json > "${GD_LOG%.log}-$1-doctor.json" || true
  fi
  kubectl get nodes -o wide >> "$GD_LOG" 2>&1 || true
  kubectl get pods -A -o wide --field-selector status.phase!=Running,status.phase!=Succeeded >> "$GD_LOG" 2>&1 || true
}

# Probe the public API once a second for N seconds and log failures.
# PROBE_CURL_OPTS adds curl flags, e.g. for the lab's MetalLB IP and
# self-signed certificate: "-k --resolve api.satark.example:443:192.168.56.240".
probe() {
  local seconds="${1:-60}" url="${PROBE_URL:-}"
  [[ -z "$url" ]] && { event "probe skipped (PROBE_URL not set)"; return 0; }
  local fail=0 i
  for ((i = 0; i < seconds; i++)); do
    # shellcheck disable=SC2086
    curl -fsS --max-time 2 ${PROBE_CURL_OPTS:-} -o /dev/null "$url" || fail=$((fail + 1))
    sleep 1
  done
  event "probe ${url}: ${fail}/${seconds} failed"
}

usage() {
  echo "usage: $0 inject|observe|revert" >&2
  exit 2
}
