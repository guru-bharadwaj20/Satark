#!/usr/bin/env bash
# Deploy the Satark MVP (gateway-api, text-analyzer, qr-decoder, telegram-bot)
# with Helm, then run the smoke tests.
#
# Prerequisites: the data layer, ingress and gateway from gitops/platform applied to the cluster.
# Usage: NAMESPACE=satark-dev IMAGE_TAG=<sha> TELEGRAM_BOT_TOKEN=... scripts/deploy-mvp.sh
# On the Vagrant lab without GHCR images, run IMAGE_TAG=lab
# scripts/load-lab-images.sh first and deploy with IMAGE_TAG=lab.
set -euo pipefail

NAMESPACE="${NAMESPACE:-satark-dev}"
IMAGE_TAG="${IMAGE_TAG:-}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICES=(text-analyzer qr-decoder gateway-api telegram-bot)

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "${NAMESPACE}" app.kubernetes.io/part-of=satark \
  pod-security.kubernetes.io/enforce=restricted --overwrite

secret_exists() { kubectl -n "${NAMESPACE}" get secret "$1" >/dev/null 2>&1; }

# Database URL built from the CNPG app credentials (never printed).
if ! secret_exists gateway-api-db; then
  user="$(kubectl -n satark-data get secret satark-pg-app -o jsonpath='{.data.username}' | base64 -d)"
  pass="$(kubectl -n satark-data get secret satark-pg-app -o jsonpath='{.data.password}' | base64 -d)"
  kubectl -n "${NAMESPACE}" create secret generic gateway-api-db \
    --from-literal=url="postgres://${user}:${pass}@satark-pg-rw.satark-data.svc:5432/satark?sslmode=require"
  unset user pass
fi

# HMAC key: generated once per environment and kept for its lifetime.
if ! secret_exists gateway-api-hmac; then
  kubectl -n "${NAMESPACE}" create secret generic gateway-api-hmac \
    --from-literal=key="$(openssl rand -base64 32)"
fi

if ! secret_exists valkey-auth; then
  pw="$(kubectl -n satark-data get secret valkey-auth -o jsonpath='{.data.password}' | base64 -d)"
  kubectl -n "${NAMESPACE}" create secret generic valkey-auth --from-literal=password="${pw}"
  unset pw
fi

if ! secret_exists telegram-bot; then
  : "${TELEGRAM_BOT_TOKEN:?set TELEGRAM_BOT_TOKEN for the first deploy}"
  kubectl -n "${NAMESPACE}" create secret generic telegram-bot --from-literal=token="${TELEGRAM_BOT_TOKEN}"
fi

for svc in "${SERVICES[@]}"; do
  echo "==> ${svc}"
  helm dependency build "${ROOT}/charts/${svc}" >/dev/null
  args=(upgrade --install "${svc}" "${ROOT}/charts/${svc}" --namespace "${NAMESPACE}"
    --wait --timeout 5m --atomic)
  [[ -n "${IMAGE_TAG}" ]] && args+=(--set "image.tag=${IMAGE_TAG}")
  helm "${args[@]}"
done

kubectl -n "${NAMESPACE}" get deploy,pods,svc,httproute
NAMESPACE="${NAMESPACE}" "${ROOT}/scripts/smoke-test.sh"
