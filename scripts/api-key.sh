#!/usr/bin/env bash
# Issue, list and revoke API keys for third-party clients of gateway-api.
#
#   scripts/api-key.sh issue  <client-name> <contact-email> [per_minute] [per_day]
#   scripts/api-key.sh list
#   scripts/api-key.sh revoke <key-prefix>
#
# Only the SHA-256 of a key is stored; the plaintext key is printed once and
# must be sent to the client over a private channel. gateway-api looks keys
# up by hash (X-API-Key header) and enforces the per-key quotas.
#
# Env:
#   PSQL      command used to run SQL (default: kubectl exec into satark-pg-1)
#   DRY_RUN=1 print SQL instead of running it
set -euo pipefail

PSQL="${PSQL:-kubectl -n satark-data exec -i satark-pg-1 -c postgres -- psql -v ON_ERROR_STOP=1 -q -d satark}"

schema="CREATE TABLE IF NOT EXISTS api_keys (
  id          BIGSERIAL PRIMARY KEY,
  prefix      TEXT        NOT NULL UNIQUE,
  key_hash    TEXT        NOT NULL UNIQUE,
  client_name TEXT        NOT NULL,
  contact     TEXT        NOT NULL,
  per_minute  INTEGER     NOT NULL DEFAULT 30,
  per_day     INTEGER     NOT NULL DEFAULT 1000,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at  TIMESTAMPTZ
);"

die() { echo "error: $*" >&2; exit 1; }

sql() {
  if [[ "${DRY_RUN:-0}" == "1" ]]; then
    cat
  else
    $PSQL
  fi
}

# Escape a value for a single-quoted SQL literal.
q() { printf "'%s'" "${1//\'/\'\'}"; }

cmd="${1:-}"; shift || true
case "$cmd" in
  issue)
    [[ $# -ge 2 ]] || die "usage: $0 issue <client-name> <contact-email> [per_minute] [per_day]"
    name="$1"; contact="$2"; pm="${3:-30}"; pd="${4:-1000}"
    [[ "$pm" =~ ^[0-9]+$ && "$pd" =~ ^[0-9]+$ ]] || die "quotas must be integers"
    [[ "$contact" == *@* ]] || die "contact must be an email address"
    # 32 chars from [A-Za-z0-9] (~190 bits of entropy).
    secret="$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32 || true)"
    [[ ${#secret} -eq 32 ]] || die "could not generate a key"
    key="sk_live_${secret}"
    prefix="${key:0:12}"
    hash="$(printf '%s' "$key" | sha256sum | cut -d' ' -f1)"
    sql <<SQL
$schema
INSERT INTO api_keys (prefix, key_hash, client_name, contact, per_minute, per_day)
VALUES ($(q "$prefix"), $(q "$hash"), $(q "$name"), $(q "$contact"), $pm, $pd);
SQL
    {
      echo
      echo "API key for $name (shown once, send it privately):"
      echo
      echo "  $key"
      echo
      echo "Prefix (for support and revocation): $prefix"
      echo "Quota: $pm/minute, $pd/day"
    } >&2
    ;;
  list)
    sql <<SQL
$schema
SELECT prefix, client_name, per_minute, per_day, created_at::date AS created,
       CASE WHEN revoked_at IS NULL THEN 'active' ELSE 'revoked' END AS state
FROM api_keys ORDER BY created_at;
SQL
    ;;
  revoke)
    [[ $# -eq 1 ]] || die "usage: $0 revoke <key-prefix>"
    [[ "$1" =~ ^sk_live_[A-Za-z0-9]{4}$ ]] || die "prefix looks like sk_live_XXXX"
    sql <<SQL
UPDATE api_keys SET revoked_at = now() WHERE prefix = $(q "$1") AND revoked_at IS NULL;
SQL
    echo "revoked $1 (gateway-api caches keys for up to 60s)" >&2
    ;;
  *)
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac
