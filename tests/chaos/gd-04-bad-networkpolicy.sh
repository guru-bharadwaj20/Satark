#!/usr/bin/env bash
# Game day 4: apply a bad NetworkPolicy to url-intel.
# Applied directly with kubectl (not through Git) so Argo CD will show the
# app as OutOfSync only if the policy is in a tracked namespace; part of the
# exercise is noticing that drift.
source "$(dirname "$0")/lib.sh"
POLICY="$(dirname "$0")/gd-04-bad-networkpolicy.yaml"

case "${1:-}" in
  inject)
    require_context
    confirm "This blocks all ingress to url-intel and its DNS egress."
    snapshot before
    event "inject: apply gd04-url-intel-lockdown"
    kubectl apply -f "$POLICY"
    probe 300
    ;;
  observe)
    kubectl -n satark get networkpolicy,ciliumnetworkpolicy | tee -a "$GD_LOG"
    if command -v hubble >/dev/null; then
      hubble observe -n satark --to-label app.kubernetes.io/name=url-intel --verdict DROPPED --last 20 | tee -a "$GD_LOG"
    fi
    ;;
  revert)
    event "revert: delete gd04 policy"
    kubectl delete -f "$POLICY" --ignore-not-found
    snapshot after
    ;;
  *) usage ;;
esac
