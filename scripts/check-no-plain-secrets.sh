#!/usr/bin/env bash
# Fail if any committed Kubernetes manifest is a plain `kind: Secret` with
# data (task 9.12). Secrets must be committed as SealedSecret resources.
# Helm templates are allowed to render Secrets from SealedSecret-provided
# values only via `existingSecret`, so charts/*/templates are checked too.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

status=0
while IFS= read -r -d '' f; do
  if grep -Eq '^kind:[[:space:]]*Secret[[:space:]]*$' "$f" \
     && grep -Eq '^(data|stringData):' "$f"; then
    echo "plain Secret with data found: $f" >&2
    status=1
  fi
done < <(find gitops charts infra policies -type f \( -name '*.yaml' -o -name '*.yml' \) \
          -not -path '*/tests/*' -print0 2>/dev/null)

[ "$status" -eq 0 ] && echo "OK: no plain Secrets committed"
exit "$status"
