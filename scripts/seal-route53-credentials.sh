#!/usr/bin/env bash
# Create an access key for the cert-manager DNS-01 IAM user and write it as
# a SealedSecret for gitops/platform/cert-manager/issuers.
#
# Usage: scripts/seal-route53-credentials.sh <iam-user-name> [aws-profile]
# Requires: aws, kubectl, kubeseal, and a cluster running sealed-secrets.
set -euo pipefail

user="${1:?usage: $0 <iam-user-name> [aws-profile]}"
profile="${2:-satark-admin}"
out="gitops/platform/cert-manager/issuers/route53-credentials.sealedsecret.yaml"

for bin in aws kubectl kubeseal jq; do
  command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 1; }
done

existing=$(aws iam list-access-keys --user-name "$user" --profile "$profile" --query 'length(AccessKeyMetadata)' --output text)
if [[ "$existing" -ge 2 ]]; then
  echo "User $user already has 2 access keys; delete an old one first." >&2
  exit 1
fi

key_json=$(aws iam create-access-key --user-name "$user" --profile "$profile" --output json)
key_id=$(jq -r .AccessKey.AccessKeyId <<<"$key_json")
secret=$(jq -r .AccessKey.SecretAccessKey <<<"$key_json")
unset key_json

kubectl create secret generic route53-credentials \
  --namespace cert-manager \
  --from-literal=access-key-id="$key_id" \
  --from-literal=secret-access-key="$secret" \
  --dry-run=client -o yaml |
  kubeseal --format yaml >"$out"
unset secret

echo "Wrote $out (key $key_id). Uncomment it in issuers/kustomization.yaml and commit."
