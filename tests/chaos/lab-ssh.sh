#!/usr/bin/env bash
# SSH into a Vagrant lab node by name. Takes the same form as
# `vagrant ssh NODE -- CMD...`, so it can stand in for $SSH in the game-day
# scripts:
#   SSH=tests/chaos/lab-ssh.sh NODE=worker-1 tests/chaos/gd-06-runtime-failure.sh observe
set -euo pipefail
node="${1:?usage: lab-ssh.sh NODE [-- CMD...]}"
shift
[[ "${1:-}" == "--" ]] && shift
case "$node" in
  cp1 | cp-1) ip=192.168.56.10 ;;
  worker-1) ip=192.168.56.11 ;;
  worker-2) ip=192.168.56.12 ;;
  *) ip="$node" ;;
esac
KEY="${LAB_SSH_KEY:-$HOME/.ssh/satark_ed25519}"
exec ssh -q -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 \
  -i "$KEY" "vagrant@$ip" "$*"
