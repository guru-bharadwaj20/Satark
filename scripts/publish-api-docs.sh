#!/usr/bin/env bash
# Publish the public API spec to docs/api/ so GitHub Pages (or any static
# host) can serve docs/api/index.html.
#
#   scripts/publish-api-docs.sh          # copy + lint
#   scripts/publish-api-docs.sh --check  # fail if docs/api is out of date (CI)
#
# Source of truth: api/openapi.yaml (gateway-api). The reputation service's
# internal spec lives next to it in docs/api/reputation.openapi.yaml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
src="$root/api/openapi.yaml"
dst="$root/docs/api/openapi.yaml"

if [[ ! -f "$src" ]]; then
  echo "error: $src not found" >&2
  exit 1
fi

lint() {
  if command -v npx >/dev/null 2>&1; then
    npx --yes @redocly/cli@1.25.11 lint --format=stylish "$@"
  else
    echo "warning: npx not found, skipping lint" >&2
  fi
}

if [[ "${1:-}" == "--check" ]]; then
  if ! cmp -s "$src" "$dst"; then
    echo "docs/api/openapi.yaml is out of date; run scripts/publish-api-docs.sh" >&2
    exit 1
  fi
  lint "$dst" "$root/docs/api/reputation.openapi.yaml"
  exit 0
fi

lint "$src" "$root/docs/api/reputation.openapi.yaml"
cp "$src" "$dst"
echo "published $dst"
