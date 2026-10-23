#!/usr/bin/env bash
# Points satark-pg's WAL archive at an in-cluster SeaweedFS S3 endpoint so the
# PITR test can run on the on-prem lab, and takes a base backup.
# Undo with `setup.sh --teardown`.
#
#   tests/chaos/pitr/onprem/setup.sh
#   ENDPOINT_URL=http://seaweedfs.satark-backup:9000 \
#     BUCKET_PATH=s3://satark-backups/cnpg/satark-pg DB=satark \
#     tests/chaos/pitr/pitr-test.sh
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
NS=satark-data
ENDPOINT=http://seaweedfs.satark-backup:9000

if [[ "${1:-}" == "--teardown" ]]; then
  kubectl -n "$NS" patch cluster satark-pg --type=json \
    -p '[{"op":"remove","path":"/spec/backup"}]' || true
  kubectl -n "$NS" delete backup -l pitr-lab=true --ignore-not-found
  kubectl -n "$NS" delete secret satark-pg-s3 --ignore-not-found
  kubectl delete -f "$HERE/s3.yaml" --ignore-not-found
  exit 0
fi

kubectl apply -f "$HERE/s3.yaml"
if ! kubectl -n satark-backup get secret s3-config >/dev/null 2>&1; then
  user="satark$(openssl rand -hex 6)"; pass="$(openssl rand -hex 24)"
  kubectl -n satark-backup create secret generic s3-config --from-literal=s3.json="$(cat <<JSON
{"identities":[{"name":"cnpg","credentials":[{"accessKey":"$user","secretKey":"$pass"}],
  "actions":["Admin","Read","Write","List","Tagging"]}]}
JSON
)"
  kubectl -n "$NS" delete secret satark-pg-s3 --ignore-not-found
  kubectl -n "$NS" create secret generic satark-pg-s3 \
    --from-literal=ACCESS_KEY_ID="$user" --from-literal=ACCESS_SECRET_KEY="$pass" \
    --from-literal=REGION=us-east-1
fi
kubectl -n satark-backup rollout status deploy/seaweedfs --timeout=10m

# Bucket, created with the shell that ships in the image.
for _ in $(seq 1 12); do
  kubectl -n satark-backup exec deploy/seaweedfs -- sh -c \
    'echo "s3.bucket.list" | weed shell -master=localhost:9333 | grep -q satark-backups ||
     echo "s3.bucket.create -name satark-backups" | weed shell -master=localhost:9333' && break
  sleep 5
done

kubectl -n "$NS" patch cluster satark-pg --type=merge -p "$(cat <<JSON
{"spec":{"backup":{"retentionPolicy":"7d","barmanObjectStore":{
  "destinationPath":"s3://satark-backups/cnpg/satark-pg",
  "endpointURL":"$ENDPOINT",
  "s3Credentials":{
    "accessKeyId":{"name":"satark-pg-s3","key":"ACCESS_KEY_ID"},
    "secretAccessKey":{"name":"satark-pg-s3","key":"ACCESS_SECRET_KEY"},
    "region":{"name":"satark-pg-s3","key":"REGION"}},
  "wal":{"compression":"gzip","maxParallel":4},
  "data":{"compression":"gzip","jobs":2}}}}}
JSON
)"

# Wait for WAL archiving to start working before the first base backup.
ok=""
for _ in $(seq 1 60); do
  ok=$(kubectl -n "$NS" get cluster satark-pg \
    -o jsonpath='{.status.conditions[?(@.type=="ContinuousArchiving")].status}')
  [[ "$ok" == True ]] && break
  sleep 5
done
[[ "$ok" == True ]] || { echo "WAL archiving did not start" >&2; exit 1; }

# Segments closed just before the patch reached the instance are reported as
# archived without being uploaded, and a backup that starts in one of them
# cannot be restored. Switch WAL and wait for a segment from after the patch.
primary=$(kubectl -n "$NS" get cluster satark-pg -o jsonpath='{.status.currentPrimary}')
seg=$(kubectl -n "$NS" exec "$primary" -c postgres -- \
  psql -At -c "SELECT pg_walfile_name(pg_switch_wal());")
for _ in $(seq 1 60); do
  last=$(kubectl -n "$NS" exec "$primary" -c postgres -- \
    psql -At -c "SELECT last_archived_wal FROM pg_stat_archiver;")
  [[ -n "$last" && ! "$last" < "$seg" ]] && break
  sleep 5
done
[[ -n "$last" && ! "$last" < "$seg" ]] || { echo "segment $seg not archived" >&2; exit 1; }

name="satark-pg-lab-$(date -u +%Y%m%d%H%M%S)"
kubectl -n "$NS" apply -f - <<YAML
apiVersion: postgresql.cnpg.io/v1
kind: Backup
metadata:
  name: $name
  labels: {pitr-lab: "true"}
spec:
  method: barmanObjectStore
  cluster: {name: satark-pg}
YAML
kubectl -n "$NS" wait --for=jsonpath='{.status.phase}'=completed "backup/$name" --timeout=20m
echo "Base backup $name completed"
