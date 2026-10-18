#!/usr/bin/env bash
# Game day 1: lose a worker node (hard power-off, no drain).
#   NODE=worker-2 VAGRANT_DIR=infra/vagrant tests/chaos/gd-01-lose-worker.sh inject
source "$(dirname "$0")/lib.sh"
NODE="${NODE:-worker-2}"
VAGRANT_DIR="${VAGRANT_DIR:-$GD_ROOT/infra/vagrant}"
# On Windows the lab VMs belong to WSL: VAGRANT="wsl -d Ubuntu-24.04 -- vagrant"
VAGRANT="${VAGRANT:-vagrant}"

case "${1:-}" in
  inject)
    require_context
    confirm "This hard-stops $NODE without draining."
    snapshot before
    event "inject: power off $NODE"
    (cd "$VAGRANT_DIR" && $VAGRANT halt --force "$NODE")
    probe 600
    ;;
  observe)
    snapshot "observe-$(date -u +%H%M%S)"
    kubectl get node "$NODE" -o jsonpath='{.status.conditions[?(@.type=="Ready")]}{"\n"}' | tee -a "$GD_LOG"
    kubectl get pods -A -o wide --field-selector "spec.nodeName=$NODE" | tee -a "$GD_LOG"
    ;;
  revert)
    event "revert: power on $NODE"
    (cd "$VAGRANT_DIR" && $VAGRANT up "$NODE")
    kubectl wait --for=condition=Ready "node/$NODE" --timeout=10m
    event "node Ready"
    snapshot after
    ;;
  *) usage ;;
esac
