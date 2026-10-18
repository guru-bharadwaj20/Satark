#!/usr/bin/env bash
# Game day 6: container runtime failure on one worker.
#   MODE=daemon (default): containerd is stopped, then masked so nothing can
#     start it. With the unit's KillMode=process the shims survive, so
#     running containers keep running, but the kubelet's PLEG goes unhealthy,
#     the node turns NotReady and nothing new can start there.
#   MODE=everything: `systemctl mask --now`. Masking first swaps the unit for
#     /dev/null with default settings (KillMode=control-group), so the stop
#     kills every shim and container on the node too (GD-06 finding).
#   NODE=worker-1 tests/chaos/gd-06-runtime-failure.sh inject
source "$(dirname "$0")/lib.sh"
NODE="${NODE:-worker-1}"
MODE="${MODE:-daemon}"
SSH="${SSH:-ssh}"   # e.g. SSH="vagrant ssh" with VAGRANT_CWD set

# Send the script on stdin: ssh joins arguments into one remote command line,
# so quotes inside $1 would not survive.
remote() { $SSH "$NODE" -- sudo bash -s <<<"$1"; }

case "${1:-}" in
  inject)
    require_context
    confirm "This stops and masks containerd on $NODE ($MODE)."
    snapshot before
    if [[ "$MODE" == everything ]]; then
      event "inject: systemctl mask --now containerd on $NODE (kills all containers)"
      remote "systemctl mask --now containerd"
    else
      event "inject: systemctl stop, then mask, containerd on $NODE"
      remote "systemctl stop containerd && systemctl mask containerd"
    fi
    probe 600
    ;;
  observe)
    kubectl get node "$NODE" -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.message}{"\n"}{end}' | tee -a "$GD_LOG"
    remote "systemctl is-active containerd; echo shims: \$(pgrep -c -f containerd-shim); journalctl -u kubelet --since -5min --no-pager | grep -iE 'PLEG|container runtime' | tail -5" | tee -a "$GD_LOG" || true
    ;;
  revert)
    event "revert: unmask and start containerd on $NODE"
    remote "systemctl unmask containerd && systemctl start containerd && systemctl restart kubelet"
    kubectl wait --for=condition=Ready "node/$NODE" --timeout=5m
    snapshot after
    ;;
  *) usage ;;
esac
