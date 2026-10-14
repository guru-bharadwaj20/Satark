#!/usr/bin/env bash
# Weekly product KPIs from Prometheus: weekly active users, scans and the
# helpful-rating rate. Prints a Markdown table (append it to
# docs/launch/kpi-log.md) or JSON with --json.
#
#   PROM_URL=http://localhost:9090 scripts/kpi-report.sh
#   kubectl -n monitoring port-forward svc/prometheus-operated 9090 &   # if needed
#
# Metric contract (see docs/launch/kpis.md):
#   satark_active_users{window="7d"}             gauge, distinct hashed users (HyperLogLog in Valkey)
#   satark_checks_total{channel,kind}            counter, one per completed check
#   satark_feedback_total{rating="helpful"|"not_helpful"}  counter, one per button press
set -euo pipefail

PROM_URL="${PROM_URL:-http://localhost:9090}"
command -v jq >/dev/null || { echo "error: jq is required" >&2; exit 1; }

query() {
  curl -fsS --get "$PROM_URL/api/v1/query" --data-urlencode "query=$1" \
    | jq -r '.data.result[0].value[1] // "0"'
}

wau=$(query 'max(satark_active_users{window="7d"})')
scans=$(query 'sum(increase(satark_checks_total[7d]))')
helpful=$(query 'sum(increase(satark_feedback_total{rating="helpful"}[7d]))')
rated=$(query 'sum(increase(satark_feedback_total[7d]))')
by_channel=$(curl -fsS --get "$PROM_URL/api/v1/query" \
  --data-urlencode 'query=sum by (channel) (increase(satark_checks_total[7d]))' \
  | jq -r '[.data.result[] | "\(.metric.channel // "unknown"): \(.value[1] | tonumber | round)"] | join(", ")')

rate=$(awk -v h="$helpful" -v r="$rated" 'BEGIN { if (r > 0) printf "%.1f", 100 * h / r; else print "n/a" }')
round() { awk -v x="$1" 'BEGIN { printf "%d", x + 0.5 }'; }
week="$(date -u +%G-W%V)"

if [[ "${1:-}" == "--json" ]]; then
  jq -n --arg week "$week" --arg wau "$(round "$wau")" --arg scans "$(round "$scans")" \
    --arg rated "$(round "$rated")" --arg rate "$rate" --arg ch "$by_channel" \
    '{week: $week, weekly_active_users: ($wau|tonumber), scans: ($scans|tonumber),
      ratings: ($rated|tonumber), helpful_rate_pct: $rate, scans_by_channel: $ch}'
  exit 0
fi

echo "| Week | WAU | Scans | Ratings | Helpful % | Scans by channel |"
echo "|---|---|---|---|---|---|"
echo "| $week | $(round "$wau") | $(round "$scans") | $(round "$rated") | $rate | $by_channel |"
