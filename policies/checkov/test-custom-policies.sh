#!/usr/bin/env bash
# Checks the custom Satark Checkov policies against fixtures:
# tests/fail must produce exactly the expected failures, and tests/pass none.
set -euo pipefail
cd "$(dirname "$0")"

checks=CKV2_SATARK_1,CKV_SATARK_2
run() {
  checkov -d "$1" --framework terraform --external-checks-dir custom \
    --check "$checks" -o json --quiet 2>/dev/null || true
}

failed_ids() {
  python3 -c '
import json, sys
data = json.load(sys.stdin)
data = data if isinstance(data, list) else [data]
for r in data:
    for f in r.get("results", {}).get("failed_checks", []):
        print(f["check_id"], f["resource"])
' | sort
}

expected_fail=$(cat <<'EOF'
CKV2_SATARK_1 aws_s3_bucket.untagged
CKV_SATARK_2 aws_security_group.inline_ssh
CKV_SATARK_2 aws_security_group_rule.range_ssh
CKV_SATARK_2 aws_vpc_security_group_ingress_rule.v6_ssh
EOF
)

actual_fail=$(run tests/fail | failed_ids)
if [[ "$actual_fail" != "$expected_fail" ]]; then
  echo "tests/fail: unexpected results" >&2
  diff <(echo "$expected_fail") <(echo "$actual_fail") >&2 || true
  exit 1
fi

actual_pass=$(run tests/pass | failed_ids)
if [[ -n "$actual_pass" ]]; then
  echo "tests/pass: unexpected failures:" >&2
  echo "$actual_pass" >&2
  exit 1
fi
echo "custom checkov policies OK"
