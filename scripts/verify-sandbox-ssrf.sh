#!/usr/bin/env bash
# Verify the sandbox SSRF block with Hubble (task 8.20).
#
# Creates LinkScans aimed at internal targets (cloud metadata, the Kubernetes
# API, a cluster service, an RFC 1918 address) plus one public control URL,
# then checks with Hubble that every internal flow was DROPPED by policy and
# that the public one was FORWARDED.
#
# Prereqs: kubectl context on the cluster, the `hubble` CLI with
#   `cilium hubble port-forward &` running, linkscan-operator deployed.
# Usage: scripts/verify-sandbox-ssrf.sh [--report docs/compliance/sandbox-ssrf-YYYY-MM-DD.md]
set -euo pipefail

NS=satark-sandbox
RUN="ssrf-$(date +%s)"
REPORT=""
[[ "${1:-}" == "--report" ]] && REPORT="${2:?report path}"

api_ip=$(kubectl get svc kubernetes -n default -o jsonpath='{.spec.clusterIP}')
nats_ip=$(kubectl get svc nats -n satark-data -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo 10.96.0.10)

declare -A TARGETS=(
  [metadata]="http://169.254.169.254/latest/meta-data/"
  [apiserver]="https://${api_ip}/"
  [nats]="http://${nats_ip}:8222/"
  [rfc1918]="http://192.168.1.1/"
)
declare -A IPS=(
  [metadata]=169.254.169.254
  [apiserver]=$api_ip
  [nats]=$nats_ip
  [rfc1918]=192.168.1.1
)
CONTROL="https://example.com/"

since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
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

fail=0
rows=""
for name in "${!IPS[@]}"; do
  ip="${IPS[$name]}"
  drops=$(hubble observe --namespace "$NS" --since "$since" --to-ip "$ip" --verdict DROPPED -o json | wc -l)
  fwd=$(hubble observe --namespace "$NS" --since "$since" --to-ip "$ip" --verdict FORWARDED -o json | wc -l)
  status=PASS
  if [[ "$drops" -eq 0 || "$fwd" -ne 0 ]]; then status=FAIL; fail=1; fi
  printf '%-10s %-16s dropped=%-4s forwarded=%-4s %s\n' "$name" "$ip" "$drops" "$fwd" "$status"
  rows+="| ${name} | ${ip} | ${drops} | ${fwd} | ${status} |"$'\n'
done
control_phase=$(kubectl get linkscan -n "$NS" "${RUN}-control" -o jsonpath='{.status.phase}')
echo "control    example.com      phase=${control_phase}"
[[ "$control_phase" == "Succeeded" ]] || { echo "control scan did not succeed; egress to the internet may be broken"; fail=1; }

if [[ -n "$REPORT" ]]; then
  cat > "$REPORT" <<EOF
# Sandbox SSRF verification, $(date -u +%Y-%m-%d)

Run id: \`${RUN}\`, cluster: \`$(kubectl config current-context)\`

| Target | IP | Hubble DROPPED | Hubble FORWARDED | Result |
|---|---|---|---|---|
${rows}
Control scan (\`${CONTROL}\`): **${control_phase}**

Overall: **$([[ $fail -eq 0 ]] && echo PASS || echo FAIL)**
EOF
  echo "report written to $REPORT"
fi

kubectl delete linkscans -n "$NS" -l "satark.io/ssrf-check=${RUN}" --wait=false >/dev/null
exit "$fail"
