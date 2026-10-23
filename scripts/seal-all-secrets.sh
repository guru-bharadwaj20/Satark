#!/usr/bin/env bash
# Seal every secret in gitops/envs/secrets-inventory.yaml for one environment
#. Values come from a local, git-ignored directory with one
# <name>.env file (KEY=VALUE lines) per secret.
#
#   scripts/seal-all-secrets.sh dev ~/satark-secrets/dev
#
# Output: gitops/envs/<env>/secrets/<name>.sealed.yaml plus a kustomization.
set -euo pipefail

ENV_NAME="${1:?usage: $0 <dev|prod> <values-dir>}"
VALUES_DIR="${2:?usage: $0 <dev|prod> <values-dir>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INVENTORY="$ROOT/gitops/envs/secrets-inventory.yaml"
OUT_DIR="$ROOT/gitops/envs/$ENV_NAME/secrets"

command -v yq >/dev/null || { echo "yq (mikefarah, v4) is required" >&2; exit 1; }
mkdir -p "$OUT_DIR"

missing=0
resources=()
count="$(yq '.secrets | length' "$INVENTORY")"
for i in $(seq 0 $((count - 1))); do
  name="$(yq ".secrets[$i].name" "$INVENTORY")"
  ns="$(yq ".secrets[$i].namespace" "$INVENTORY")"
  src="$VALUES_DIR/$name.env"
  if [ ! -f "$src" ]; then
    echo "MISSING $src" >&2
    missing=1
    continue
  fi
  # Every key listed in the inventory must be present in the env file.
  for key in $(yq ".secrets[$i].keys[]" "$INVENTORY"); do
    grep -q "^${key}=" "$src" || { echo "MISSING key $key in $src" >&2; missing=1; }
  done
  "$ROOT/scripts/seal-secret.sh" "$ns" "$name" "$OUT_DIR/$name.sealed.yaml" < "$src"
  resources+=("$name.sealed.yaml")
done

{
  echo "apiVersion: kustomize.config.k8s.io/v1beta1"
  echo "kind: Kustomization"
  echo "resources:"
  for r in "${resources[@]}"; do echo "  - $r"; done
} > "$OUT_DIR/kustomization.yaml"

[ "$missing" -eq 0 ] || { echo "Some secrets were not sealed, see above." >&2; exit 1; }
echo "Sealed ${#resources[@]} secrets into $OUT_DIR"
