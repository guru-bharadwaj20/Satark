#!/usr/bin/env bash
# Bring up the Satark on-prem lab from scratch:
#   1. vagrant up      (cp-1, worker-1, worker-2 on 192.168.56.0/24)
#   2. site.yml        (hardened kubeadm cluster with Cilium and add-ons)
#
# Usage:
#   ./scripts/bootstrap.sh                 # full build
#   ./scripts/bootstrap.sh --skip-vagrant  # re-run Ansible only
#   SATARK_WORKERS=1 ./scripts/bootstrap.sh
#   ./scripts/bootstrap.sh -- --tags addons   # extra args go to ansible-playbook
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VAGRANT_DIR="$REPO_ROOT/infra/vagrant"
ANSIBLE_DIR="$REPO_ROOT/infra/ansible"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/satark_ed25519}"
KUBECONFIG_OUT="$HOME/.kube/satark-onprem.conf"
SKIP_VAGRANT=0
ANSIBLE_ARGS=()

log() { printf '\n==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-vagrant) SKIP_VAGRANT=1; shift ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    --) shift; ANSIBLE_ARGS=("$@"); break ;;
    *) die "unknown argument: $1" ;;
  esac
done

for cmd in vagrant ansible-playbook ansible-galaxy ssh; do
  command -v "$cmd" >/dev/null || die "$cmd not found (run scripts/setup-control-node.sh first)"
done
[[ -f "$SSH_KEY" ]] || die "SSH key $SSH_KEY missing (run scripts/setup-control-node.sh first)"

export SATARK_SSH_PUBKEY="${SSH_KEY}.pub"
# Under WSL, Vagrant must store VM state on the Windows filesystem.
# The host-only network is not reachable from WSL2, so Ansible connects
# through the forwarded SSH ports instead.
ON_WSL=0
if grep -qi microsoft /proc/version 2>/dev/null; then
  ON_WSL=1
  export VAGRANT_WSL_ENABLE_WINDOWS_ACCESS=1
fi

if [[ "$SKIP_VAGRANT" -eq 0 ]]; then
  log "Starting VMs with Vagrant"
  (cd "$VAGRANT_DIR" && vagrant up --provider=virtualbox)
fi

log "Installing Ansible collections"
ansible-galaxy collection install -r "$ANSIBLE_DIR/requirements.yml" >/dev/null

cd "$ANSIBLE_DIR"
export ANSIBLE_CONFIG="$ANSIBLE_DIR/ansible.cfg"
if [[ "$ON_WSL" -eq 1 ]]; then
  export ANSIBLE_INVENTORY="inventory/onprem/hosts.yml,inventory/onprem/wsl-ports.yml"
fi

log "Waiting for SSH on all nodes"
for attempt in $(seq 1 30); do
  if ansible k8s_cluster -m ansible.builtin.ping -o >/dev/null 2>&1; then
    break
  fi
  [[ "$attempt" -eq 30 ]] && die "nodes did not become reachable over SSH"
  sleep 5
done

log "Running playbooks/site.yml"
ansible-playbook playbooks/site.yml "${ANSIBLE_ARGS[@]}"

log "Cluster is up"
echo "export KUBECONFIG=$KUBECONFIG_OUT"
echo "kubectl get nodes -o wide"
echo "Verify with: ./scripts/verify-cluster.sh"
