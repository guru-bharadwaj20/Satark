"""Unit tests for file_issues.py (no network, no gh calls)."""

import importlib.util
import sys
from pathlib import Path

spec = importlib.util.spec_from_file_location("file_issues", Path(__file__).with_name("file_issues.py"))
fi = importlib.util.module_from_spec(spec)
sys.modules["file_issues"] = fi
spec.loader.exec_module(fi)

REPORT = {
    "Controls": [
        {
            "node_type": "master",
            "tests": [
                {
                    "section": "1.2",
                    "desc": "API Server",
                    "results": [
                        {"test_number": "1.2.16", "test_desc": "Ensure --profiling is false",
                         "status": "FAIL", "scored": True, "remediation": "Set --profiling=false"},
                        {"test_number": "1.2.2", "test_desc": "Ensure basic auth off",
                         "status": "PASS", "scored": True, "remediation": ""},
                        {"test_number": "1.2.10", "test_desc": "Admission EventRateLimit",
                         "status": "WARN", "scored": False, "remediation": "Optional"},
                    ],
                }
            ],
        }
    ]
}


def test_only_fail_by_default():
    found = fi.parse_report(REPORT)
    assert [f.check_id for f in found] == ["1.2.16"]
    assert found[0].severity == "high"


def test_include_warn_sorted_numerically():
    found = fi.parse_report(REPORT, include_warn=True)
    assert [f.check_id for f in found] == ["1.2.10", "1.2.16"]
    assert found[0].severity == "medium"


def test_dedupe_by_prefix():
    f = fi.parse_report(REPORT)[0]
    assert fi.already_filed(f, {"[kube-bench 1.2.16] old wording"})
    assert not fi.already_filed(f, {"[kube-bench 1.2.1] other"})


def test_body_contains_remediation():
    assert "Set --profiling=false" in fi.parse_report(REPORT)[0].body()
