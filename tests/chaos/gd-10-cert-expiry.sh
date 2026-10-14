#!/usr/bin/env bash
# Game day 10: certificate expiry drill.
# Works in a throwaway namespace so production certificates are never touched:
#  1. a TLS Secret holding a certificate that expires in 1 day
#  2. a cert-manager Certificate whose issuer does not exist (renewal broken)
#  3. a short-lived (1h) Certificate from a self-signed issuer, to watch
#     cert-manager renew it automatically
# Also records kubeadm control-plane certificate expiry.
source "$(dirname "$0")/lib.sh"
NS=gd10-certs
CP="${CP:-cp1}"
SSH="${SSH:-ssh}"

case "${1:-}" in
  inject)
    require_context
    snapshot before
    kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f -
    tmp=$(mktemp -d)
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 \
      -subj "/CN=gd10-expiring.satark.example" -keyout "$tmp/tls.key" -out "$tmp/tls.crt" 2>/dev/null
    kubectl -n "$NS" create secret tls gd10-expiring --cert="$tmp/tls.crt" --key="$tmp/tls.key" \
      --dry-run=client -o yaml | kubectl apply -f -
    rm -rf "$tmp"
    event "inject: secret gd10-expiring (expires in 1 day)"
    kubectl apply -f - <<YAML
apiVersion: cert-manager.io/v1
kind: Issuer
metadata: {name: gd10-selfsigned, namespace: $NS}
spec: {selfSigned: {}}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: gd10-short-lived, namespace: $NS}
spec:
  secretName: gd10-short-lived
  commonName: gd10-short.satark.example
  dnsNames: [gd10-short.satark.example]
  duration: 1h
  renewBefore: 50m
  issuerRef: {name: gd10-selfsigned, kind: Issuer}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: gd10-broken-issuer, namespace: $NS}
spec:
  secretName: gd10-broken-issuer
  dnsNames: [gd10-broken.satark.example]
  issuerRef: {name: does-not-exist, kind: ClusterIssuer}
YAML
    event "inject: certificates gd10-short-lived and gd10-broken-issuer"
    ;;
  observe)
    kubectl -n "$NS" get certificates,certificaterequests | tee -a "$GD_LOG"
    kubectl satark doctor -n "$NS" | tee -a "$GD_LOG" || true
    $SSH "$CP" -- sudo kubeadm certs check-expiration | tee -a "$GD_LOG" || true
    ;;
  revert)
    event "revert: delete namespace $NS"
    kubectl delete namespace "$NS" --wait=true
    snapshot after
    ;;
  *) usage ;;
esac
