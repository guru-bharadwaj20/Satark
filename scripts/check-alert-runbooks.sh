#!/usr/bin/env bash
# Fail if any alerting rule under gitops/ lacks a runbook_url annotation, or
# points at a runbook file that does not exist in docs/runbooks (task 6.14).
# Requires: yq v4.
set -euo pipefail
cd "$(dirname "$0")/.."

status=0
while IFS= read -r file; do
  missing=$(yq -r '.spec.groups[].rules[] | select(has("alert")) | select(.annotations.runbook_url == null) | .alert' "$file")
  if [[ -n "$missing" ]]; then
    echo "::error file=$file::alerts without runbook_url: $missing"
    status=1
  fi
  while IFS= read -r url; do
    [[ -z "$url" ]] && continue
    path="docs/runbooks/${url##*/docs/runbooks/}"
    if [[ ! -f "$path" ]]; then
      echo "::error file=$file::runbook not found: $path"
      status=1
    fi
  done < <(yq -r '.spec.groups[].rules[] | select(has("alert")) | .annotations.runbook_url // ""' "$file" | sort -u)
done < <(grep -rl 'kind: PrometheusRule' gitops/)

exit "$status"
