#!/usr/bin/env bash
# Game day 4: apply a bad NetworkPolicy to url-intel.
# Applied directly with kubectl (not through Git) so Argo CD will show the
# app as OutOfSync only if the policy is in a tracked namespace; part of the
# exercise is noticing that drift.
source "$(dirname "$0")/lib.sh"
POLICY="$(dirname "$0")/gd-04-bad-networkpolicy.yaml"
# The lab has no url-intel: NS=satark-dev TARGET=text-analyzer PORT=8000
NS="${NS:-satark}"
TARGET="${TARGET:-url-intel}"
PORT="${PORT:-8081}"

policy() {
  sed -e "s/namespace: satark$/namespace: $NS/" \
      -e "s/name: url-intel$/name: $TARGET/" \
      -e "s/port: 8081$/port: $PORT/" "$POLICY"
}

case "${1:-}" in
  inject)
    require_context
    confirm "This blocks all ingress to $TARGET and its DNS egress."
    snapshot before
    event "inject: apply gd04-url-intel-lockdown to $NS/$TARGET"
    policy | kubectl apply -f -
    probe 300
    ;;
  observe)
    kubectl -n "$NS" get networkpolicy,ciliumnetworkpolicy | tee -a "$GD_LOG"
    if command -v hubble >/dev/null; then
      hubble observe -n "$NS" --to-label "app.kubernetes.io/name=$TARGET" --verdict DROPPED --last 20 | tee -a "$GD_LOG"
    fi
    ;;
  revert)
    event "revert: delete gd04 policy"
    policy | kubectl delete -f - --ignore-not-found
    snapshot after
    ;;
  *) usage ;;
esac
