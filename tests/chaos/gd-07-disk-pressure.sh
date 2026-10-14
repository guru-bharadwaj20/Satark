#!/usr/bin/env bash
# Game day 7: disk pressure on a node.
# Fills the filesystem holding /var/lib/containerd until free space is below
# the kubelet's hard eviction threshold (default nodefs.available<10%).
#   NODE=worker-2 TARGET_FREE_PCT=7 tests/chaos/gd-07-disk-pressure.sh inject
source "$(dirname "$0")/lib.sh"
NODE="${NODE:-worker-2}"
SSH="${SSH:-ssh}"
TARGET_FREE_PCT="${TARGET_FREE_PCT:-7}"
FILLER=/var/lib/containerd/gd07-filler

remote() { $SSH "$NODE" -- sudo bash -c "'$1'"; }

case "${1:-}" in
  inject)
    require_context
    confirm "This fills the disk on $NODE to ${TARGET_FREE_PCT}% free."
    snapshot before
    # size = used_needed - currently_used, computed on the node
    remote "set -e; read size avail < <(df -B1 --output=size,avail /var/lib/containerd | tail -1); \
      want=\$(( size * $TARGET_FREE_PCT / 100 )); fill=\$(( avail - want )); \
      [ \$fill -gt 0 ] && fallocate -l \$fill $FILLER; df -h /var/lib/containerd"
    event "inject: filler written on $NODE"
    probe 300
    ;;
  observe)
    kubectl get node "$NODE" -o jsonpath='{.status.conditions[?(@.type=="DiskPressure")]}{"\n"}' | tee -a "$GD_LOG"
    kubectl get node "$NODE" -o jsonpath='{.spec.taints}{"\n"}' | tee -a "$GD_LOG"
    kubectl get pods -A --field-selector "spec.nodeName=$NODE,status.phase=Failed" | tee -a "$GD_LOG"
    remote "df -h /var/lib/containerd /var/log" | tee -a "$GD_LOG" || true
    ;;
  revert)
    event "revert: remove filler on $NODE"
    remote "rm -f $FILLER; df -h /var/lib/containerd"
    # Evicted pods stay as Failed objects; clean them up.
    kubectl delete pods -A --field-selector "spec.nodeName=$NODE,status.phase=Failed" --ignore-not-found
    until [[ "$(kubectl get node "$NODE" -o jsonpath='{.status.conditions[?(@.type=="DiskPressure")].status}')" == False ]]; do sleep 10; done
    event "DiskPressure cleared"
    snapshot after
    ;;
  *) usage ;;
esac
