#!/usr/bin/env bash
# Game day 6: container runtime failure on one worker.
# containerd is stopped and masked so systemd cannot restart it. Running
# containers keep running (shims survive) but the kubelet's PLEG goes
# unhealthy, the node turns NotReady and nothing new can start there.
#   NODE=worker-1 tests/chaos/gd-06-runtime-failure.sh inject
source "$(dirname "$0")/lib.sh"
NODE="${NODE:-worker-1}"
SSH="${SSH:-ssh}"   # e.g. SSH="vagrant ssh" with VAGRANT_CWD set

remote() { $SSH "$NODE" -- sudo bash -c "'$1'"; }

case "${1:-}" in
  inject)
    require_context
    confirm "This stops and masks containerd on $NODE."
    snapshot before
    event "inject: systemctl mask --now containerd on $NODE"
    remote "systemctl mask --now containerd"
    probe 600
    ;;
  observe)
    kubectl get node "$NODE" -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.message}{"\n"}{end}' | tee -a "$GD_LOG"
    remote "systemctl is-active containerd; journalctl -u kubelet --since -5min --no-pager | grep -iE 'PLEG|container runtime' | tail -5" | tee -a "$GD_LOG" || true
    ;;
  revert)
    event "revert: unmask and start containerd on $NODE"
    remote "systemctl unmask containerd && systemctl start containerd && systemctl restart kubelet"
    kubectl wait --for=condition=Ready "node/$NODE" --timeout=5m
    snapshot after
    ;;
  *) usage ;;
esac
