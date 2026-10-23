#!/usr/bin/env bash
# Seal a secret for GitOps. Reads values from env-style KEY=VALUE
# lines on stdin so they never appear in shell history or process args.
#
#   scripts/seal-secret.sh <namespace> <name> <output.yaml> < secret.env
#
# Uses the cluster controller by default. Offline sealing: set
# SEALED_SECRETS_CERT=path/to/pub-cert.pem (fetch with
# `kubeseal --fetch-cert > pub-cert.pem`; the cert is public and safe to commit).
set -euo pipefail

if [ $# -ne 3 ]; then
  echo "usage: $0 <namespace> <name> <output.yaml> < secret.env" >&2
  exit 2
fi
NS="$1" NAME="$2" OUT="$3"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/secret.env"

CERT_ARGS=()
if [ -n "${SEALED_SECRETS_CERT:-}" ]; then
  CERT_ARGS=(--cert "$SEALED_SECRETS_CERT")
fi

kubectl create secret generic "$NAME" -n "$NS" \
  --from-env-file="$TMP/secret.env" --dry-run=client -o yaml \
  | kubeseal --format yaml --scope strict \
      --controller-name sealed-secrets-controller --controller-namespace kube-system \
      "${CERT_ARGS[@]}" > "$OUT"

echo "Wrote $OUT (SealedSecret $NS/$NAME). Commit it; never commit secret.env."
