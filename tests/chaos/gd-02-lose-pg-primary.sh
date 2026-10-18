#!/usr/bin/env bash
# Game day 2: lose the Postgres primary (delete the primary pod, then optionally
# its PVC to force a full replica rebuild).
#   tests/chaos/gd-02-lose-pg-primary.sh inject [--with-pvc]
source "$(dirname "$0")/lib.sh"
NS=satark-data
CLUSTER=satark-pg

primary() {
  kubectl -n "$NS" get cluster "$CLUSTER" -o jsonpath='{.status.currentPrimary}'
}

case "${1:-}" in
  inject)
    require_context
    p="$(primary)"
    confirm "This deletes the CNPG primary pod $p${2:+ and its PVC}."
    snapshot before
    event "primary before: $p"
    if [[ "${2:-}" == "--with-pvc" ]]; then
      # Delete the WAL PVC too. If it is left behind, CNPG recreates the pod
      # against the missing data PVC and it sits Pending forever (GD-02 finding).
      event "inject: delete PVCs $p and $p-wal (and pod)"
      kubectl -n "$NS" delete pvc "$p" --wait=false
      kubectl -n "$NS" delete pvc "$p-wal" --wait=false --ignore-not-found
    fi
    event "inject: delete pod $p"
    kubectl -n "$NS" delete pod "$p" --grace-period=0 --force
    start=$(date +%s)
    until [[ "$(primary)" != "$p" && -n "$(primary)" ]]; do sleep 1; done
    event "new primary $(primary) after $(( $(date +%s) - start ))s"
    probe 180
    ;;
  observe)
    kubectl -n "$NS" get cluster "$CLUSTER" | tee -a "$GD_LOG"
    kubectl -n "$NS" get pods -l "cnpg.io/cluster=$CLUSTER" -L cnpg.io/instanceRole | tee -a "$GD_LOG"
    if kubectl cnpg version >/dev/null 2>&1; then kubectl cnpg status "$CLUSTER" -n "$NS" | tee -a "$GD_LOG"; fi
    ;;
  revert)
    # CNPG self-heals; revert only waits for the full instance count.
    want=$(kubectl -n "$NS" get cluster "$CLUSTER" -o jsonpath='{.spec.instances}')
    until [[ "$(kubectl -n "$NS" get cluster "$CLUSTER" -o jsonpath='{.status.readyInstances}')" == "$want" ]]; do sleep 5; done
    event "all $want instances ready; primary $(primary)"
    snapshot after
    ;;
  *) usage ;;
esac
