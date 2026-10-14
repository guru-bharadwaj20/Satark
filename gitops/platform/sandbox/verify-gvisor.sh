#!/usr/bin/env bash
# Verify that pods using the gvisor RuntimeClass really run under runsc.
# Usage: gitops/platform/sandbox/verify-gvisor.sh [namespace]
set -euo pipefail
NS="${1:-satark-sandbox}"
POD="gvisor-check-$RANDOM"

kubectl get runtimeclass gvisor -o jsonpath='{.handler}{"\n"}'
kubectl -n "$NS" run "$POD" --image=busybox:1.36 --restart=Never \
  --overrides='{"spec":{"runtimeClassName":"gvisor","automountServiceAccountToken":false,
  "securityContext":{"runAsNonRoot":true,"runAsUser":10001,"seccompProfile":{"type":"RuntimeDefault"}},
  "containers":[{"name":"c","image":"busybox:1.36","command":["dmesg"],
  "securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}},
  "resources":{"limits":{"cpu":"100m","memory":"64Mi"}}}]}}'
trap 'kubectl -n "$NS" delete pod "$POD" --ignore-not-found >/dev/null' EXIT
kubectl -n "$NS" wait --for=jsonpath='{.status.phase}'=Succeeded "pod/$POD" --timeout=120s
if kubectl -n "$NS" logs "$POD" | grep -q "gVisor"; then
  echo "OK: pod ran under gVisor"
else
  echo "FAIL: dmesg does not show the gVisor kernel" >&2
  exit 1
fi
