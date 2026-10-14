#!/usr/bin/env bash
# Turn a WSL2 Ubuntu distribution into the Ansible control node for the
# Satark on-prem lab. Idempotent: safe to re-run.
#
# What it does:
#   * installs Python, pipx, kubectl, helm and the Vagrant Windows interop env
#   * installs ansible-core + ansible-lint into an isolated pipx environment
#   * installs the Ansible collections from infra/ansible/requirements.yml
#   * generates an ed25519 SSH key used by Ansible to reach the VMs
#   * writes /etc/wsl.conf so Windows-mounted files get proper permissions
#
# Usage (inside WSL Ubuntu, from the repo root):
#   ./scripts/setup-control-node.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANSIBLE_CORE_VERSION="${ANSIBLE_CORE_VERSION:-2.17.*}"
KUBECTL_MINOR="${KUBECTL_MINOR:-v1.35}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/satark_ed25519}"

log() { printf '==> %s\n' "$*"; }

if ! grep -qi microsoft /proc/version 2>/dev/null; then
  log "Warning: this does not look like WSL; continuing anyway"
fi

log "Installing base packages"
sudo apt-get update -qq
sudo apt-get install -y -qq python3 python3-venv pipx git curl gnupg \
  ca-certificates openssh-client jq rsync

log "Configuring /etc/wsl.conf (metadata on /mnt/c, systemd)"
if ! grep -q 'options = "metadata' /etc/wsl.conf 2>/dev/null; then
  sudo tee /etc/wsl.conf >/dev/null <<'EOF'
[boot]
systemd = true

[automount]
options = "metadata,umask=22,fmask=11"
EOF
  log "wsl.conf updated: run 'wsl --shutdown' from Windows to apply"
fi

log "Installing ansible-core ${ANSIBLE_CORE_VERSION} and ansible-lint via pipx"
pipx ensurepath >/dev/null
pipx install --force "ansible-core==${ANSIBLE_CORE_VERSION}" >/dev/null
pipx inject ansible-core netaddr jmespath kubernetes >/dev/null
pipx install --force ansible-lint >/dev/null
export PATH="$HOME/.local/bin:$PATH"

log "Installing Ansible collections"
ansible-galaxy collection install -r "$REPO_ROOT/infra/ansible/requirements.yml"

log "Installing kubectl ${KUBECTL_MINOR} and helm"
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL "https://pkgs.k8s.io/core:/stable:/${KUBECTL_MINOR}/deb/Release.key" \
  | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/${KUBECTL_MINOR}/deb/ /" \
  | sudo tee /etc/apt/sources.list.d/kubernetes.list >/dev/null
sudo apt-get update -qq
sudo apt-get install -y -qq kubectl
if ! command -v helm >/dev/null; then
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi

log "Making the Windows Vagrant binary usable from WSL"
if ! grep -q VAGRANT_WSL_ENABLE_WINDOWS_ACCESS "$HOME/.bashrc"; then
  cat >>"$HOME/.bashrc" <<'EOF'

# Satark: drive the Windows VirtualBox/Vagrant install from WSL
export VAGRANT_WSL_ENABLE_WINDOWS_ACCESS=1
export PATH="$PATH:/mnt/c/Program Files/Oracle/VirtualBox"
EOF
fi

if [[ ! -f "$SSH_KEY" ]]; then
  log "Generating SSH key $SSH_KEY"
  ssh-keygen -t ed25519 -N '' -C "satark-ansible" -f "$SSH_KEY"
fi

log "Control node ready"
ansible --version | head -n1
echo "Next: ./scripts/bootstrap.sh"
