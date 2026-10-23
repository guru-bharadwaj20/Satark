#!/usr/bin/env bash
# Verify the sandbox SSRF block with Hubble.
#
# Creates LinkScans aimed at internal targets (cloud metadata, the Kubernetes
# API, an in-cluster service, an RFC 1918 address) plus one public control
# URL, then checks with Hubble that every internal flow was DROPPED by policy
# and that the public one was FORWARDED.
#
# Prereqs: kubectl context on the cluster, the `hubble` CLI with the relay on
#   localhost:4245 (`cilium hubble port-forward &` or
#   `kubectl -n kube-system port-forward svc/hubble-relay 4245:80 &`),
#   linkscan-operator deployed.
# Usage: scripts/verify-sandbox-ssrf.sh [--report docs/compliance/sandbox-ssrf-YYYY-MM-DD.md]
#   HUBBLE_SERVER overrides the relay address.
set -euo pipefail

NS=satark-sandbox
RUN="ssrf-$(date +%s)"
REPORT=""
[[ "${1:-}" == "--report" ]] && REPORT="${2:?report path}"
HUBBLE=(hubble observe --server "${HUBBLE_SERVER:-localhost:4245}" --namespace "$NS")

api_ip=$(kubectl get svc kubernetes -n default -o jsonpath='{.spec.clusterIP}')

# In-cluster HTTP service: NATS monitoring when NATS runs, else the gateway API.
svc_name="" svc_ip="" svc_port=""
for candidate in satark-data/nats satark-dev/gateway-api satark/gateway-api; do
  if ip=$(kubectl get svc "${candidate#*/}" -n "${candidate%/*}" -o jsonpath='{.spec.clusterIP}' 2>/dev/null); then
    svc_name="$candidate" svc_ip="$ip"
    if [[ "$candidate" == */nats ]]; then
      svc_port=8222
    else
      svc_port=$(kubectl get svc "${candidate#*/}" -n "${candidate%/*}" -o jsonpath='{.spec.ports[0].port}')
    fi
    break
  fi
done
[[ -n "$svc_ip" ]] || { echo "no in-cluster service found to target" >&2; exit 2; }

declare -A TARGETS=(
  [metadata]="http://169.254.169.254/latest/meta-data/"
  [apiserver]="https://${api_ip}/"
  [service]="http://${svc_ip}:${svc_port}/"
  [rfc1918]="http://192.168.1.1/"
)
declare -A IPS=(
  [metadata]=169.254.169.254
  [apiserver]=$api_ip
  [service]=$svc_ip
  [rfc1918]=192.168.1.1
)
CONTROL="https://example.com/"

# Record flows while the scans run. Hubble's ring buffer can hold only a
# minute or two of flows on a busy node, so querying afterwards with --since
# can miss the early drops.
FLOWS="$(mktemp)"
trap 'kill "${hubble_pid:-}" 2>/dev/null || true; rm -f "$FLOWS"' EXIT
"${HUBBLE[@]}" --follow -o json > "$FLOWS" &
hubble_pid=$!
sleep 2

for name in "${!TARGETS[@]}" control; do
  url="${TARGETS[$name]:-$CONTROL}"
  kubectl apply -f - <<EOF
apiVersion: scan.satark.io/v1alpha1
kind: LinkScan
metadata:
  name: ${RUN}-${name}
  namespace: ${NS}
  labels: {satark.io/ssrf-check: "${RUN}"}
spec:
  url: "${url}"
  timeoutSeconds: 30
  ttlSecondsAfterFinished: 600
EOF
done

echo "waiting for scans to finish..."
for _ in $(seq 1 60); do
  pending=$(kubectl get linkscans -n "$NS" -l "satark.io/ssrf-check=${RUN}" \
    -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}' | grep -cvE 'Succeeded|Failed|TimedOut' || true)
  [[ "$pending" == "0" ]] && break
  sleep 5
done
kubectl get linkscans -n "$NS" -l "satark.io/ssrf-check=${RUN}"
sleep 3
kill "$hubble_pid" 2>/dev/null || true
wait "$hubble_pid" 2>/dev/null || true

# Count flows sent to an IP with a given verdict (one JSON flow per line).
count() {
  grep -F "\"destination\":\"$1\"" "$FLOWS" | grep -cF "\"verdict\":\"$2\"" || true
}

fail=0
rows=""
for name in metadata apiserver service rfc1918; do
  ip="${IPS[$name]}"
  drops=$(count "$ip" DROPPED)
  fwd=$(count "$ip" FORWARDED)
  phase=$(kubectl get linkscan -n "$NS" "${RUN}-${name}" -o jsonpath='{.status.phase}')
  status=PASS
  if [[ "$drops" -eq 0 || "$fwd" -ne 0 ]]; then status=FAIL; fail=1; fi
  printf '%-10s %-16s dropped=%-4s forwarded=%-4s scan=%-10s %s\n' "$name" "$ip" "$drops" "$fwd" "$phase" "$status"
  rows+="| ${name} | \`${TARGETS[$name]}\` | ${drops} | ${fwd} | ${phase} | ${status} |"$'\n'
done
control_phase=$(kubectl get linkscan -n "$NS" "${RUN}-control" -o jsonpath='{.status.phase}')
echo "control    example.com      phase=${control_phase}"
[[ "$control_phase" == "Succeeded" ]] || { echo "control scan did not succeed; egress to the internet may be broken"; fail=1; }

if [[ -n "$REPORT" ]]; then
  cat > "$REPORT" <<EOF
# Sandbox SSRF verification, $(date -u +%Y-%m-%d)

Run id: \`${RUN}\`, cluster: \`$(kubectl config current-context)\`,
in-cluster service target: \`${svc_name}\`

| Target | URL | Hubble DROPPED | Hubble FORWARDED | Scan phase | Result |
|---|---|---|---|---|---|
${rows}
Control scan (\`${CONTROL}\`): **${control_phase}**

Overall: **$([[ $fail -eq 0 ]] && echo PASS || echo FAIL)**
EOF
  echo "report written to $REPORT"
fi

kubectl delete linkscans -n "$NS" -l "satark.io/ssrf-check=${RUN}" --wait=false >/dev/null
exit "$fail"
