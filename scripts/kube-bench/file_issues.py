#!/usr/bin/env python3
"""File kube-bench FAIL findings as GitHub issues (task 9.9).

Reads a kube-bench JSON report (from scripts/kube-bench/run.sh) and creates
one issue per failing check, labelled `security` and `kube-bench`. Checks that
already have an open issue (matched by the "[kube-bench <id>]" title prefix)
are skipped, so the script is safe to re-run after every benchmark.

Dry-run is the default; pass --apply to actually create issues via `gh`.

    scripts/kube-bench/file_issues.py report.json              # preview
    scripts/kube-bench/file_issues.py report.json --apply      # create
    scripts/kube-bench/file_issues.py report.json --include-warn --apply
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

LABELS = ("security", "kube-bench")
# CIS "scored"/automated checks with FAIL are treated as high severity.
HIGH_STATUSES = {"FAIL"}


@dataclass(frozen=True)
class Finding:
    check_id: str
    text: str
    status: str
    section: str
    node_type: str
    remediation: str
    scored: bool

    @property
    def title(self) -> str:
        return f"[kube-bench {self.check_id}] {self.text}"[:250]

    @property
    def severity(self) -> str:
        return "high" if self.status in HIGH_STATUSES and self.scored else "medium"

    def body(self) -> str:
        return (
            f"**CIS check:** `{self.check_id}` ({self.section}, node type `{self.node_type}`)\n"
            f"**Status:** {self.status}  \n**Scored:** {self.scored}  \n"
            f"**Severity:** {self.severity}\n\n"
            f"### Finding\n{self.text}\n\n"
            f"### Remediation\n{self.remediation.strip() or '_none provided_'}\n\n"
            "### Done when\n- [ ] Fix applied through Ansible or GitOps (no hand edits)\n"
            "- [ ] `scripts/kube-bench/run.sh` shows this check as PASS\n"
            "- [ ] Or a documented exception is added to `docs/compliance/`\n"
        )


def parse_report(data: dict, include_warn: bool = False) -> list[Finding]:
    wanted = {"FAIL", "WARN"} if include_warn else {"FAIL"}
    # kube-bench emits either {"Controls": [...]} or a bare list of controls.
    controls = data.get("Controls", []) if isinstance(data, dict) else data
    findings: dict[str, Finding] = {}
    for control in controls or []:
        node_type = control.get("node_type", "")
        for group in control.get("tests", []) or []:
            section = f"{group.get('section', '')} {group.get('desc', '')}".strip()
            for result in group.get("results", []) or []:
                status = result.get("status", "")
                if status not in wanted:
                    continue
                f = Finding(
                    check_id=str(result.get("test_number", "?")),
                    text=str(result.get("test_desc", "")).strip(),
                    status=status,
                    section=section,
                    node_type=node_type,
                    remediation=str(result.get("remediation", "")),
                    scored=bool(result.get("scored", False)),
                )
                findings.setdefault(f.check_id, f)
    return sorted(findings.values(), key=lambda f: [int(p) if p.isdigit() else p for p in f.check_id.split(".")])


def open_issue_titles(repo: str | None) -> set[str]:
    cmd = ["gh", "issue", "list", "--state", "open", "--label", "kube-bench",
           "--limit", "500", "--json", "title"]
    if repo:
        cmd += ["--repo", repo]
    out = subprocess.run(cmd, check=True, capture_output=True, text=True).stdout
    return {i["title"] for i in json.loads(out or "[]")}


def already_filed(f: Finding, titles: set[str]) -> bool:
    prefix = f"[kube-bench {f.check_id}]"
    return any(t.startswith(prefix) for t in titles)


def create_issue(f: Finding, repo: str | None) -> None:
    cmd = ["gh", "issue", "create", "--title", f.title, "--body", f.body()]
    for label in (*LABELS, f"severity:{f.severity}"):
        cmd += ["--label", label]
    if repo:
        cmd += ["--repo", repo]
    subprocess.run(cmd, check=True)


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("report", type=Path, help="kube-bench JSON report")
    ap.add_argument("--repo", help="owner/repo (default: current gh repo)")
    ap.add_argument("--include-warn", action="store_true", help="also file WARN results")
    ap.add_argument("--apply", action="store_true", help="create issues (default is dry-run)")
    args = ap.parse_args(argv)

    findings = parse_report(json.loads(args.report.read_text()), args.include_warn)
    titles = open_issue_titles(args.repo) if args.apply else set()

    created = skipped = 0
    for f in findings:
        if already_filed(f, titles):
            skipped += 1
            print(f"skip   {f.check_id} (open issue exists)")
            continue
        if args.apply:
            create_issue(f, args.repo)
            print(f"filed  {f.check_id} [{f.severity}] {f.text}")
        else:
            print(f"would  {f.check_id} [{f.severity}] {f.text}")
        created += 1
    mode = "created" if args.apply else "to create (dry-run)"
    print(f"{len(findings)} findings: {created} {mode}, {skipped} skipped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
