#!/usr/bin/env bash
# Game day 4: apply a bad NetworkPolicy to url-intel.
# Applied directly with kubectl (not through Git) so Argo CD will show the
# app as OutOfSync only if the policy is in a tracked namespace; part of the
# exercise is noticing that drift.
#   MODE=add     - a second, separate policy (the default). NetworkPolicies
#                  are additive, so while the chart's own policy allows the
#                  traffic this one cannot block it.
#   MODE=replace - overwrite the chart's own policy (named after the app) with
#                  the bad spec, as a bad networkPolicy values change would.
source "$(dirname "$0")/lib.sh"
POLICY="$(dirname "$0")/gd-04-bad-networkpolicy.yaml"
# The lab has no url-intel: NS=satark-dev TARGET=text-analyzer PORT=8000
NS="${NS:-satark}"
TARGET="${TARGET:-url-intel}"
PORT="${PORT:-8081}"
MODE="${MODE:-add}"
BACKUP="${GD_LOG%.log}-networkpolicy.yaml"

policy() {
  local name=gd04-url-intel-lockdown
  [[ "$MODE" == replace ]] && name="$TARGET"
  sed -e "s/namespace: satark$/namespace: $NS/"       -e "s/name: gd04-url-intel-lockdown$/name: $name/" \
      -e "s/name: url-intel$/name: $TARGET/" \
      -e "s/port: 8081$/port: $PORT/" "$POLICY"
}

case "${1:-}" in
  inject)
    require_context
    confirm "This blocks all ingress to $TARGET and its DNS egress."
    snapshot before
    if [[ "$MODE" == replace ]]; then
      kubectl -n "$NS" get networkpolicy "$TARGET" -o yaml         | grep -v -E '^  (resourceVersion|uid|creationTimestamp):' > "$BACKUP"
      event "inject: replace networkpolicy $NS/$TARGET with the bad spec"
      policy | kubectl replace -f -
    else
      event "inject: apply gd04-url-intel-lockdown to $NS/$TARGET"
      policy | kubectl apply -f -
    fi
    probe 300
    ;;
  observe)
    kubectl -n "$NS" get networkpolicy,ciliumnetworkpolicy | tee -a "$GD_LOG"
    if command -v hubble >/dev/null; then
      hubble observe -n "$NS" --to-label "app.kubernetes.io/name=$TARGET" --verdict DROPPED --last 20 | tee -a "$GD_LOG"
    fi
    ;;
  revert)
    if [[ "$MODE" == replace ]]; then
      event "revert: restore networkpolicy $NS/$TARGET"
      kubectl replace -f "$BACKUP"
    else
      event "revert: delete gd04 policy"
      policy | kubectl delete -f - --ignore-not-found
    fi
    snapshot after
    ;;
  *) usage ;;
esac
