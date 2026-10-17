#!/usr/bin/env bash
# Build the MVP images locally and import them into containerd on every lab
# node, so the on-prem lab can run the charts without pulling from GHCR.
# The charts use pullPolicy IfNotPresent, so the imported tag is used as is.
#
# Needs docker and the Vagrant VMs running. Run from the Windows host (Git
# Bash) or WSL/Linux.
#
# Usage: IMAGE_TAG=lab scripts/load-lab-images.sh [service...]
#        then IMAGE_TAG=lab scripts/deploy-mvp.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE_TAG="${IMAGE_TAG:-lab}"
PREFIX="${IMAGE_PREFIX:-ghcr.io/guru-bharadwaj20/satark}"
read -r -a NODES <<< "${NODES:-cp-1 worker-1 worker-2}"
SERVICES=("$@")
[[ ${#SERVICES[@]} -eq 0 ]] && SERVICES=(text-analyzer qr-decoder gateway-api telegram-bot)

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

images=()
for svc in "${SERVICES[@]}"; do
  img="${PREFIX}/${svc}:${IMAGE_TAG}"
  echo "==> build ${img}"
  # text-analyzer bundles rules/, so its context is the repository root.
  if [[ "${svc}" == text-analyzer ]]; then
    docker build -q --provenance=false -t "${img}" -f "${ROOT}/apps/${svc}/Dockerfile" "${ROOT}"
  else
    docker build -q --provenance=false -t "${img}" --build-arg VERSION="${IMAGE_TAG}" "${ROOT}/apps/${svc}"
  fi
  images+=("${img}")
done

docker save -o "${WORK}/images.tar" "${images[@]}"

# Reach the nodes through their fixed forwarded SSH ports (Vagrantfile) with
# the per-machine Vagrant keys. `vagrant ssh-config` is not used: Windows
# Vagrant refuses to touch VMs that WSL Vagrant created.
declare -A PORT=([cp-1]=2222 [worker-1]=2200 [worker-2]=2201)
for node in "${NODES[@]}"; do
  cat >> "${WORK}/ssh_config" <<EOF
Host ${node}
  HostName 127.0.0.1
  Port ${PORT[${node}]}
  User vagrant
  IdentityFile ${ROOT}/infra/vagrant/.vagrant/machines/${node}/virtualbox/private_key
  IdentitiesOnly yes
  BatchMode yes
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  LogLevel ERROR
EOF
done
for node in "${NODES[@]}"; do
  echo "==> import into ${node}"
  scp -q -F "${WORK}/ssh_config" "${WORK}/images.tar" "${node}:/tmp/satark-images.tar"
  ssh -F "${WORK}/ssh_config" "${node}" \
    'sudo ctr -n k8s.io images import /tmp/satark-images.tar >/dev/null && rm -f /tmp/satark-images.tar'
done
echo "Imported ${#images[@]} images (tag ${IMAGE_TAG}) into ${NODES[*]}."
