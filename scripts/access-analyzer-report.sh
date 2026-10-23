#!/usr/bin/env bash
# Export active IAM Access Analyzer findings to a Markdown table.
# Requires the AWS CLI with read access (access-analyzer:List*/Get*).
#
#   scripts/access-analyzer-report.sh dev > docs/compliance/access-analyzer-$(date +%F).md
set -euo pipefail

ENVIRONMENT="${1:-dev}"
PROJECT="${PROJECT:-satark}"
REGION="${AWS_REGION:-ap-south-1}"

analyzer_arn() {
  aws accessanalyzer list-analyzers --region "$REGION" \
    --query "analyzers[?name=='$1'].arn | [0]" --output text
}

echo "# IAM Access Analyzer findings ($ENVIRONMENT, $(date -u +%F))"
echo
for kind in external unused; do
  arn="$(analyzer_arn "$PROJECT-$ENVIRONMENT-$kind")"
  if [ -z "$arn" ] || [ "$arn" = "None" ]; then
    echo "Analyzer $PROJECT-$ENVIRONMENT-$kind not found" >&2
    exit 1
  fi
  echo "## $kind access"
  echo
  echo "| Finding ID | Resource | Type | Detail | Action |"
  echo "|---|---|---|---|---|"
  aws accessanalyzer list-findings-v2 --region "$REGION" --analyzer-arn "$arn" \
    --filter '{"status":{"eq":["ACTIVE"]}}' --output json \
  | jq -r '.findings[] | "| \(.id) | \(.resource) | \(.resourceType) | \(.findingType // "external") | _fix or archive_ |"'
  echo
done
