#!/usr/bin/env python3
"""Summarise time-to-diagnose trials: opsbot vs manual.

    python3 tests/chaos/ttd/analyze.py [tests/chaos/ttd/results.csv]

Prints per-condition median, IQR, and accuracy, plus paired differences for
game days run under both conditions. Only the standard library is used.
"""
from __future__ import annotations

import csv
import statistics
import sys
from collections import defaultdict
from pathlib import Path


def quartiles(xs: list[float]) -> tuple[float, float]:
    if len(xs) < 2:
        return (xs[0], xs[0]) if xs else (float("nan"), float("nan"))
    q = statistics.quantiles(xs, n=4, method="inclusive")
    return q[0], q[2]


def load(path: Path) -> list[dict]:
    with path.open(newline="") as f:
        rows = list(csv.DictReader(f))
    for r in rows:
        r["diagnose_s"] = float(r["diagnose_s"])
        r["correct"] = r["correct"].strip().lower() == "true"
    return rows


def summarise(rows: list[dict]) -> str:
    out = []
    by_cond: dict[str, list[dict]] = defaultdict(list)
    for r in rows:
        by_cond[r["condition"]].append(r)

    out.append(f"{'condition':<10}{'n':>4}{'median_s':>10}{'q1':>8}{'q3':>8}{'accuracy':>10}")
    for cond in ("manual", "opsbot"):
        rs = by_cond.get(cond, [])
        if not rs:
            continue
        # Only correct diagnoses count toward time; wrong ones count against accuracy.
        times = [r["diagnose_s"] for r in rs if r["correct"]]
        med = statistics.median(times) if times else float("nan")
        q1, q3 = quartiles(times)
        acc = sum(r["correct"] for r in rs) / len(rs)
        out.append(f"{cond:<10}{len(rs):>4}{med:>10.0f}{q1:>8.0f}{q3:>8.0f}{acc:>9.0%}")

    paired = defaultdict(dict)
    for r in rows:
        if r["correct"]:
            paired[r["game_day"]].setdefault(r["condition"], []).append(r["diagnose_s"])
    diffs = []
    out.append("")
    out.append(f"{'game_day':<10}{'manual':>8}{'opsbot':>8}{'saved':>8}")
    for gd in sorted(paired):
        c = paired[gd]
        if "manual" in c and "opsbot" in c:
            m, o = statistics.median(c["manual"]), statistics.median(c["opsbot"])
            diffs.append(m - o)
            out.append(f"{gd:<10}{m:>8.0f}{o:>8.0f}{m - o:>8.0f}")
    if diffs:
        out.append("")
        out.append(f"median time saved per game day: {statistics.median(diffs):.0f}s over {len(diffs)} paired game days")
    return "\n".join(out)


def main() -> int:
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).with_name("results.csv")
    if not path.exists():
        print(f"no results yet at {path}", file=sys.stderr)
        return 1
    print(summarise(load(path)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
