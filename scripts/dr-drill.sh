#!/usr/bin/env bash
# Disaster-recovery drill: rebuild Satark on a temporary AWS k3s VM from S3
# backups, switch DNS, and record RTO and RPO.
#
#   ADMIN_CIDR=203.0.113.4/32 HOSTED_ZONE_ID=Z123 RECORD=api.satark.example \
#     scripts/dr-drill.sh
#
# Phases (each timestamped into $OUT/timeline.txt):
#   provision -> k3s ready -> velero restore -> postgres restore -> apps ready
#   -> dns switched -> first successful public request (= RTO end)
# RPO = outage start minus timestamp of the newest data in the restore.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TF_DIR="$ROOT/infra/terraform/envs/dr"
OUT="${OUT:-$ROOT/docs/game-days/dr-runs/$(date -u +%Y%m%dT%H%M%SZ)}"
ADMIN_CIDR="${ADMIN_CIDR:?operator public IP/32}"
HOSTED_ZONE_ID="${HOSTED_ZONE_ID:?Route 53 zone ID}"
RECORD="${RECORD:?DNS name to fail over, e.g. api.satark.example}"
BUCKET="${BUCKET:-satark-backups-prod}"
REGION="${REGION:-ap-south-1}"
BACKUP_NAME="${BACKUP_NAME:-}"          # default: newest completed daily backup
OUTAGE_START="${OUTAGE_START:-$(date -u +%s)}"  # declared start of the outage
export KUBECONFIG="$OUT/kubeconfig"

mkdir -p "$OUT"
mark() { echo "$(date -u +%s) $1" | tee -a "$OUT/timeline.txt"; }
ssm() {
  local id; id=$(terraform -chdir="$TF_DIR" output -raw dr_instance_id)
  local cmd_id
  cmd_id=$(aws ssm send-command --region "$REGION" --instance-ids "$id" \
    --document-name AWS-RunShellScript --parameters "commands=[\"$1\"]" \
    --query Command.CommandId --output text)
  aws ssm wait command-executed --region "$REGION" --command-id "$cmd_id" --instance-id "$id"
  aws ssm get-command-invocation --region "$REGION" --command-id "$cmd_id" \
    --instance-id "$id" --query StandardOutputContent --output text
}

echo "outage_start $OUTAGE_START" > "$OUT/timeline.txt"

# 1. Provision
mark provision_start
terraform -chdir="$TF_DIR" init -input=false
terraform -chdir="$TF_DIR" apply -auto-approve -input=false -var "admin_cidrs=[\"$ADMIN_CIDR\"]"
IP=$(terraform -chdir="$TF_DIR" output -raw dr_public_ip)
until ssm "test -f /var/lib/satark-dr-ready && echo ok" | grep -q ok; do sleep 15; done
ssm "cat /etc/rancher/k3s/k3s.yaml" | sed "s/127.0.0.1/$IP/" > "$KUBECONFIG"
chmod 600 "$KUBECONFIG"
mark k3s_ready

# 2. Velero in restore-only mode (instance role gives read-only S3 access)
helm repo add vmware-tanzu https://vmware-tanzu.github.io/helm-charts >/dev/null
helm upgrade --install velero vmware-tanzu/velero -n velero --create-namespace --wait \
  --version 8.1.0 \
  --set credentials.useSecret=false \
  --set deployNodeAgent=true \
  --set snapshotsEnabled=false \
  --set "initContainers[0].name=velero-plugin-for-aws" \
  --set "initContainers[0].image=velero/velero-plugin-for-aws:v1.11.0" \
  --set "initContainers[0].volumeMounts[0].mountPath=/target" \
  --set "initContainers[0].volumeMounts[0].name=plugins" \
  --set "configuration.backupStorageLocation[0].name=default" \
  --set "configuration.backupStorageLocation[0].provider=aws" \
  --set "configuration.backupStorageLocation[0].bucket=$BUCKET" \
  --set "configuration.backupStorageLocation[0].prefix=velero" \
  --set "configuration.backupStorageLocation[0].accessMode=ReadOnly" \
  --set "configuration.backupStorageLocation[0].config.region=$REGION"

until velero backup get -o json 2>/dev/null | jq -e '.items | length > 0' >/dev/null; do sleep 10; done
if [[ -z "$BACKUP_NAME" ]]; then
  BACKUP_NAME=$(velero backup get -o json | jq -r '
    [.items[] | select(.status.phase=="Completed" and (.metadata.name|startswith("daily")))]
    | sort_by(.status.completionTimestamp) | last | .metadata.name')
fi
BACKUP_TS=$(velero backup get "$BACKUP_NAME" -o json | jq -r '.status.startTimestamp')
echo "backup $BACKUP_NAME $BACKUP_TS" >> "$OUT/timeline.txt"

# 3. Restore cluster objects and file-system volumes
mark velero_restore_start
velero restore create "dr-$(date +%s)" --from-backup "$BACKUP_NAME" \
  --exclude-resources clusters.postgresql.cnpg.io,backups.postgresql.cnpg.io \
  --wait
mark velero_restore_done

# 4. Postgres: bootstrap from WAL archive to the latest available point
helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null
helm upgrade --install cnpg cnpg/cloudnative-pg -n cnpg-system --create-namespace --wait
kubectl apply -f "$ROOT/tests/chaos/dr/satark-pg-recovery.yaml"
kubectl -n satark-data wait --for=condition=Ready cluster/satark-pg --timeout=45m
mark postgres_ready
LAST_TX=$(kubectl -n satark-data exec satark-pg-1 -c postgres -- \
  psql -At -d app -c "SELECT extract(epoch from max(created_at))::bigint FROM scans" 2>/dev/null || echo "")
echo "last_tx ${LAST_TX:-unknown}" >> "$OUT/timeline.txt"

# 5. Apps
kubectl -n satark wait --for=condition=Available deploy --all --timeout=15m
mark apps_ready

# 6. DNS switch (60 s TTL)
aws route53 change-resource-record-sets --hosted-zone-id "$HOSTED_ZONE_ID" --change-batch "{
  \"Comment\": \"DR drill failover\",
  \"Changes\": [{\"Action\": \"UPSERT\", \"ResourceRecordSet\": {
    \"Name\": \"$RECORD\", \"Type\": \"A\", \"TTL\": 60,
    \"ResourceRecords\": [{\"Value\": \"$IP\"}]}}]}"
mark dns_switched

# 7. First good public request
until curl -fsS --max-time 5 "https://$RECORD/healthz" >/dev/null; do sleep 5; done
mark service_restored

restored=$(awk '$2=="service_restored"{print $1}' "$OUT/timeline.txt")
rto=$(( restored - OUTAGE_START ))
if [[ -n "${LAST_TX:-}" ]]; then rpo=$(( OUTAGE_START - LAST_TX )); else rpo=unknown; fi
cat <<EOF | tee "$OUT/summary.txt"
backup=$BACKUP_NAME
backup_started=$BACKUP_TS
rto_seconds=$rto
rpo_seconds=$rpo
dr_ip=$IP
EOF
echo "Drill complete. Switch DNS back and destroy the VM per SOP-05 section 5."
