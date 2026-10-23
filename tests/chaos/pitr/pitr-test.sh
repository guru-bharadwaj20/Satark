#!/usr/bin/env bash
# Point-in-time recovery test for the satark-pg CNPG cluster.
#
# 1. Writes a marker row, records the timestamp T, then writes a "bad" row.
# 2. Forces a WAL switch so both are archived to S3.
# 3. Bootstraps a new cluster (satark-pg-pitr) from the object store with
#    recoveryTarget.targetTime = T.
# 4. Asserts the marker row exists and the bad row does not.
#
# Usage: tests/chaos/pitr/pitr-test.sh [--keep]
# On-prem lab: run onprem/setup.sh first, then set ENDPOINT_URL, BUCKET_PATH
# and DB as it prints.
set -euo pipefail

NS="${NS:-satark-data}"
SRC="${SRC:-satark-pg}"
DST="${DST:-satark-pg-pitr}"
BUCKET_PATH="${BUCKET_PATH:-s3://satark-backups-prod/cnpg/satark-pg}"
ENDPOINT_URL="${ENDPOINT_URL:-}"   # empty = AWS S3
DB="${DB:-app}"
KEEP=false
[[ "${1:-}" == "--keep" ]] && KEEP=true

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

primary() {
  kubectl -n "$NS" get pods -l "cnpg.io/cluster=$1,cnpg.io/instanceRole=primary" \
    -o jsonpath='{.items[0].metadata.name}'
}

psql_on() {
  kubectl -n "$NS" exec -i "$(primary "$1")" -c postgres -- \
    psql -v ON_ERROR_STOP=1 -At -d "$DB" -c "$2"
}

marker="pitr-$(date +%s)"
log "Writing marker ${marker}"
psql_on "$SRC" "CREATE TABLE IF NOT EXISTS pitr_probe (id text primary key, kind text, at timestamptz default now());"
psql_on "$SRC" "INSERT INTO pitr_probe VALUES ('${marker}','good');"
sleep 2
# Postgres timestamp with an explicit offset: CNPG 1.24 turns an RFC 3339
# "...Z" into "...000000Z", which recovery_target_time rejects.
TARGET="$(psql_on "$SRC" "SELECT to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS.US') || '+00';")"
sleep 2
psql_on "$SRC" "INSERT INTO pitr_probe VALUES ('${marker}-bad','bad');"
psql_on "$SRC" "SELECT pg_switch_wal();" >/dev/null
log "Recovery target ${TARGET}; waiting 60s for WAL to reach the object store"
sleep 60
wal="$(psql_on "$SRC" "SELECT last_archived_wal FROM pg_stat_archiver;")"
log "Last archived WAL: ${wal}"

IMAGE="$(kubectl -n "$NS" get cluster "$SRC" -o jsonpath='{.spec.imageName}')"
STORAGE_CLASS="$(kubectl -n "$NS" get cluster "$SRC" -o jsonpath='{.spec.storage.storageClass}')"

log "Creating ${DST}"
kubectl -n "$NS" apply -f - <<YAML
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: ${DST}
spec:
  instances: 1
  imageName: ${IMAGE}
  storage:
    size: 10Gi
${STORAGE_CLASS:+    storageClass: ${STORAGE_CLASS}}
  bootstrap:
    recovery:
      source: origin
      database: ${DB}
      recoveryTarget:
        targetTime: "${TARGET}"
  externalClusters:
    - name: origin
      barmanObjectStore:
        destinationPath: ${BUCKET_PATH}
        serverName: ${SRC}
${ENDPOINT_URL:+        endpointURL: ${ENDPOINT_URL}}
        s3Credentials:
          accessKeyId: {name: satark-pg-s3, key: ACCESS_KEY_ID}
          secretAccessKey: {name: satark-pg-s3, key: ACCESS_SECRET_KEY}
          region: {name: satark-pg-s3, key: REGION}
        wal:
          maxParallel: 4
YAML

start=$(date +%s)
kubectl -n "$NS" wait --for=condition=Ready "cluster/${DST}" --timeout=30m
elapsed=$(( $(date +%s) - start ))

good=$(psql_on "$DST" "SELECT count(*) FROM pitr_probe WHERE id='${marker}';")
bad=$(psql_on "$DST" "SELECT count(*) FROM pitr_probe WHERE id='${marker}-bad';")

result=PASS
[[ "$good" == "1" && "$bad" == "0" ]] || result=FAIL
log "Result: ${result} (good=${good} bad=${bad}) restore_seconds=${elapsed}"

$KEEP || kubectl -n "$NS" delete cluster "$DST" --wait=false
[[ "$result" == PASS ]]
