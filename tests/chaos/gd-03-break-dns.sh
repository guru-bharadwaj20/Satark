#!/usr/bin/env bash
# Game day 3: break in-cluster DNS.
#   MODE=scale|config tests/chaos/gd-03-break-dns.sh inject
#   scale  - scale CoreDNS to 0 (total outage)
#   config - point the CoreDNS forwarder at a black-hole IP (external names fail,
#            cluster.local still resolves)
source "$(dirname "$0")/lib.sh"
MODE="${MODE:-scale}"
BACKUP="${GD_LOG%.log}-coredns-configmap.yaml"

case "${1:-}" in
  inject)
    require_context
    confirm "This breaks DNS ($MODE) for the whole cluster."
    snapshot before
    kubectl -n kube-system get configmap coredns -o yaml > "$BACKUP"
    kubectl -n kube-system get deploy coredns -o jsonpath='{.spec.replicas}' > "${BACKUP}.replicas"
    if [[ "$MODE" == scale ]]; then
      event "inject: scale coredns to 0"
      kubectl -n kube-system scale deploy coredns --replicas=0
    else
      event "inject: forward . 192.0.2.1 (TEST-NET-1, unroutable)"
      kubectl -n kube-system get configmap coredns -o json \
        | jq '.data.Corefile |= sub("forward \\. [^{\\n]*"; "forward . 192.0.2.1 ")' \
        | kubectl apply -f -
      kubectl -n kube-system rollout restart deploy coredns
    fi
    probe 300
    ;;
  observe)
    kubectl run "gd03-dnsprobe-$RANDOM" --rm -i --restart=Never --image=busybox:1.36 -- \
      sh -c 'nslookup kubernetes.default.svc.cluster.local; nslookup example.com' 2>&1 | tee -a "$GD_LOG" || true
    kubectl -n kube-system get pods -l k8s-app=kube-dns -o wide | tee -a "$GD_LOG"
    ;;
  revert)
    event "revert: restore coredns config and replicas"
    kubectl apply -f "$BACKUP"
    kubectl -n kube-system scale deploy coredns --replicas="$(cat "${BACKUP}.replicas" 2>/dev/null || echo 2)"
    kubectl -n kube-system rollout restart deploy coredns
    kubectl -n kube-system rollout status deploy coredns --timeout=5m
    snapshot after
    ;;
  *) usage ;;
esac
