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
set -euo pipefail

NS="${NS:-satark-data}"
SRC="${SRC:-satark-pg}"
DST="${DST:-satark-pg-pitr}"
BUCKET_PATH="${BUCKET_PATH:-s3://satark-backups-prod/cnpg/satark-pg}"
KEEP=false
[[ "${1:-}" == "--keep" ]] && KEEP=true

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

primary() {
  kubectl -n "$NS" get pods -l "cnpg.io/cluster=$1,cnpg.io/instanceRole=primary" \
    -o jsonpath='{.items[0].metadata.name}'
}

psql_on() {
  kubectl -n "$NS" exec -i "$(primary "$1")" -c postgres -- \
    psql -v ON_ERROR_STOP=1 -At -d app -c "$2"
}

marker="pitr-$(date +%s)"
log "Writing marker ${marker}"
psql_on "$SRC" "CREATE TABLE IF NOT EXISTS pitr_probe (id text primary key, kind text, at timestamptz default now());"
psql_on "$SRC" "INSERT INTO pitr_probe VALUES ('${marker}','good');"
sleep 2
TARGET="$(psql_on "$SRC" "SELECT to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"');")"
sleep 2
psql_on "$SRC" "INSERT INTO pitr_probe VALUES ('${marker}-bad','bad');"
psql_on "$SRC" "SELECT pg_switch_wal();" >/dev/null
log "Recovery target ${TARGET}; waiting 60s for WAL to reach S3"
sleep 60

log "Creating ${DST}"
kubectl -n "$NS" apply -f - <<YAML
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: ${DST}
spec:
  instances: 1
  storage:
    size: 10Gi
  bootstrap:
    recovery:
      source: origin
      recoveryTarget:
        targetTime: "${TARGET}"
  externalClusters:
    - name: origin
      barmanObjectStore:
        destinationPath: ${BUCKET_PATH}
        serverName: ${SRC}
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
