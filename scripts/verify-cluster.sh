#!/usr/bin/env bash
# Smoke-test the on-prem cluster after bootstrap:
#   * every node is Ready
#   * in-cluster DNS resolves kubernetes.default and an external name
#   * a test LoadBalancer Service gets an IP from the MetalLB pool and answers
#   * metrics-server serves node metrics and a default StorageClass exists
#
# Exit code is non-zero if any check fails. Test resources live in the
# satark-verify namespace and are deleted at the end.
#
# Usage: KUBECONFIG=~/.kube/satark-onprem.conf ./scripts/verify-cluster.sh
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-$HOME/.kube/satark-onprem.conf}"
export KUBECONFIG
NS=satark-verify
POOL_PREFIX="${POOL_PREFIX:-192.168.56.}"
FAILED=0

pass() { printf '  [PASS] %s\n' "$*"; }
fail() { printf '  [FAIL] %s\n' "$*"; FAILED=1; }
cleanup() { kubectl delete namespace "$NS" --ignore-not-found --wait=false >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== Nodes"
kubectl get nodes -o wide
if kubectl wait --for=condition=Ready nodes --all --timeout=120s >/dev/null; then
  pass "all $(kubectl get nodes --no-headers | wc -l) nodes Ready"
else
  fail "not all nodes are Ready"
fi

echo "== System pods"
not_running="$(kubectl get pods -A --no-headers | awk '$4 != "Running" && $4 != "Completed"' || true)"
if [[ -z "$not_running" ]]; then pass "all system pods Running"; else fail "pods not running:"; echo "$not_running"; fi

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

echo "== In-cluster DNS"
kubectl -n "$NS" run dnstest --image=busybox:1.36 --restart=Never --command -- sleep 300 >/dev/null
kubectl -n "$NS" wait --for=condition=Ready pod/dnstest --timeout=120s >/dev/null
if kubectl -n "$NS" exec dnstest -- nslookup kubernetes.default.svc.cluster.local >/dev/null 2>&1; then
  pass "kubernetes.default.svc.cluster.local resolves"
else
  fail "cluster DNS lookup failed"
fi
if kubectl -n "$NS" exec dnstest -- nslookup example.com >/dev/null 2>&1; then
  pass "external name resolves through CoreDNS"
else
  fail "external DNS lookup failed"
fi

echo "== LoadBalancer"
kubectl -n "$NS" create deployment echo --image=nginx:1.27-alpine --port=80 >/dev/null
kubectl -n "$NS" expose deployment echo --type=LoadBalancer --port=80 >/dev/null
kubectl -n "$NS" rollout status deployment/echo --timeout=120s >/dev/null
lb_ip=""
for _ in $(seq 1 30); do
  lb_ip="$(kubectl -n "$NS" get svc echo -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
  [[ -n "$lb_ip" ]] && break
  sleep 2
done
if [[ "$lb_ip" == "$POOL_PREFIX"* ]]; then
  pass "LoadBalancer got $lb_ip"
  if curl -fsS --max-time 5 "http://$lb_ip/" >/dev/null; then
    pass "LoadBalancer $lb_ip answers HTTP"
  else
    fail "LoadBalancer $lb_ip did not answer (run from a host on 192.168.56.0/24)"
  fi
else
  fail "LoadBalancer did not get an IP from the pool (got '${lb_ip}')"
fi

echo "== Metrics and storage"
if kubectl top nodes >/dev/null 2>&1; then pass "metrics-server serves node metrics"; else fail "kubectl top nodes failed"; fi
default_sc="$(kubectl get storageclass -o jsonpath='{range .items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")]}{.metadata.name}{end}')"
if [[ -n "$default_sc" ]]; then pass "default StorageClass is $default_sc"; else fail "no default StorageClass"; fi

echo
if [[ "$FAILED" -eq 0 ]]; then echo "Cluster verification PASSED"; else echo "Cluster verification FAILED"; fi
exit "$FAILED"
