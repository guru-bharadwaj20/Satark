#!/usr/bin/env python3
"""Evaluate scam classification on eval/dataset.jsonl.

Reports precision, recall and F1 per category, macro averages, and binary
scam-vs-legitimate metrics, then optionally enforces quality gates.

Examples:
    # Offline keyword baseline (no LLM needed)
    python eval/run_eval.py --provider baseline

    # Local Ollama through the production classifier
    SATARK_OLLAMA_URL=http://localhost:11434 python eval/run_eval.py --provider ollama

    # Score predictions produced elsewhere ({"id": ..., "label": ...} per line)
    python eval/run_eval.py --predictions preds.jsonl

    # CI gate
    python eval/run_eval.py --provider baseline --min-scam-recall 0.85 --max-legit-fpr 0.10
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
import time
from collections import Counter
from dataclasses import dataclass
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(REPO / "apps" / "text-analyzer"))

import baseline  # noqa: E402

LEGIT = "legitimate"


@dataclass
class Metrics:
    precision: float
    recall: float
    f1: float
    support: int


def load_jsonl(path: Path) -> list[dict]:
    with path.open(encoding="utf-8") as f:
        return [json.loads(line) for line in f if line.strip()]


def _prf(tp: int, fp: int, fn: int) -> tuple[float, float, float]:
    p = tp / (tp + fp) if tp + fp else 0.0
    r = tp / (tp + fn) if tp + fn else 0.0
    f = 2 * p * r / (p + r) if p + r else 0.0
    return p, r, f


def compute_metrics(gold: list[str], pred: list[str]) -> dict:
    labels = sorted(set(gold) | set(pred))
    per: dict[str, Metrics] = {}
    for c in labels:
        tp = sum(g == c and p == c for g, p in zip(gold, pred, strict=True))
        fp = sum(g != c and p == c for g, p in zip(gold, pred, strict=True))
        fn = sum(g == c and p != c for g, p in zip(gold, pred, strict=True))
        p, r, f = _prf(tp, fp, fn)
        per[c] = Metrics(round(p, 4), round(r, 4), round(f, 4), gold.count(c))
    present = [c for c in labels if per[c].support]
    macro = {
        k: round(sum(getattr(per[c], k) for c in present) / len(present), 4)
        for k in ("precision", "recall", "f1")
    }
    gs = [g != LEGIT for g in gold]
    ps = [p != LEGIT for p in pred]
    tp = sum(a and b for a, b in zip(gs, ps, strict=True))
    fp = sum((not a) and b for a, b in zip(gs, ps, strict=True))
    fn = sum(a and not b for a, b in zip(gs, ps, strict=True))
    bp, br, bf = _prf(tp, fp, fn)
    n_legit = sum(not a for a in gs)
    return {
        "n": len(gold),
        "accuracy": round(sum(g == p for g, p in zip(gold, pred, strict=True)) / len(gold), 4),
        "per_category": {c: vars(m) for c, m in per.items()},
        "macro": macro,
        "binary": {
            "scam_precision": round(bp, 4),
            "scam_recall": round(br, 4),
            "scam_f1": round(bf, 4),
            "legit_false_positive_rate": round(fp / n_legit, 4) if n_legit else 0.0,
        },
        "confusion": {
            f"{g}->{p}": n for (g, p), n in Counter(zip(gold, pred, strict=True)).items() if g != p
        },
    }


async def predict_llm(rows: list[dict], provider_name: str, concurrency: int) -> tuple[list, dict]:
    from app.llm.classifier import LLMClassifier
    from app.llm.prompts import PROMPT_VERSION
    from app.llm.provider import get_provider

    provider = get_provider(provider_name)
    clf = LLMClassifier(provider)
    sem = asyncio.Semaphore(concurrency)
    stats: Counter = Counter()

    async def one(row: dict) -> str:
        async with sem:
            out = await clf.classify(row["text"], language=row.get("lang", "en"))
        stats["input_tokens"] += out.input_tokens
        stats["output_tokens"] += out.output_tokens
        if out.result is None:
            stats[f"fallback_{out.fallback_reason.value}"] += 1
            return baseline.predict(row["text"])
        return out.result.category.value

    try:
        preds = await asyncio.gather(*(one(r) for r in rows))
    finally:
        await provider.aclose()
    info = {"provider": provider.name, "model": provider.model, "prompt_version": PROMPT_VERSION}
    return list(preds), {**info, **stats}


def render_markdown(report: dict) -> str:
    m = report["metrics"]
    lines = [
        f"### Evaluation: {report['run']['provider']} ({m['n']} messages)",
        "",
        f"Accuracy **{m['accuracy']:.3f}**, macro F1 **{m['macro']['f1']:.3f}**, "
        f"scam recall **{m['binary']['scam_recall']:.3f}**, scam precision "
        f"**{m['binary']['scam_precision']:.3f}**, legitimate FPR "
        f"**{m['binary']['legit_false_positive_rate']:.3f}**",
        "",
        "| Category | Precision | Recall | F1 | Support |",
        "|---|---:|---:|---:|---:|",
    ]
    for c, v in sorted(m["per_category"].items()):
        lines.append(
            f"| {c} | {v['precision']:.3f} | {v['recall']:.3f} | {v['f1']:.3f} | {v['support']} |"
        )
    if report.get("gates"):
        lines += ["", "| Gate | Threshold | Actual | Result |", "|---|---:|---:|---|"]
        for g in report["gates"]:
            ok = "pass" if g["passed"] else "**FAIL**"
            lines.append(f"| {g['name']} | {g['threshold']} | {g['actual']} | {ok} |")
    return "\n".join(lines) + "\n"


def check_gates(metrics: dict, args) -> list[dict]:
    b = metrics["binary"]
    spec = [
        ("min_scam_recall", args.min_scam_recall, b["scam_recall"], ">="),
        ("min_scam_precision", args.min_scam_precision, b["scam_precision"], ">="),
        ("min_macro_f1", args.min_macro_f1, metrics["macro"]["f1"], ">="),
        ("max_legit_fpr", args.max_legit_fpr, b["legit_false_positive_rate"], "<="),
    ]
    gates = []
    for name, thr, actual, op in spec:
        if thr is None:
            continue
        passed = actual >= thr if op == ">=" else actual <= thr
        gates.append({"name": name, "threshold": thr, "actual": actual, "passed": passed})
    return gates


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    ap.add_argument("--dataset", type=Path, default=HERE / "dataset.jsonl")
    src = ap.add_mutually_exclusive_group()
    src.add_argument(
        "--provider",
        default="baseline",
        help="baseline, or an LLM provider name (ollama, bedrock, anthropic, ...)",
    )
    src.add_argument("--predictions", type=Path, help="JSONL with id and label per line")
    ap.add_argument("--lang", choices=["en", "hi", "kn"], help="only evaluate one language")
    ap.add_argument("--concurrency", type=int, default=4)
    ap.add_argument("--out", type=Path, help="write the JSON report here")
    ap.add_argument("--markdown", type=Path, help="write a Markdown summary here")
    ap.add_argument("--min-scam-recall", type=float)
    ap.add_argument("--min-scam-precision", type=float)
    ap.add_argument("--min-macro-f1", type=float)
    ap.add_argument("--max-legit-fpr", type=float)
    args = ap.parse_args(argv)

    rows = load_jsonl(args.dataset)
    if args.lang:
        rows = [r for r in rows if r.get("lang") == args.lang]
    if not rows:
        ap.error("dataset is empty")

    started = time.perf_counter()
    run: dict = {"dataset": str(args.dataset.name), "lang": args.lang or "all"}
    if args.predictions:
        by_id = {p["id"]: p["label"] for p in load_jsonl(args.predictions)}
        missing = [r["id"] for r in rows if r["id"] not in by_id]
        if missing:
            ap.error(f"{len(missing)} rows have no prediction, e.g. {missing[:3]}")
        preds = [by_id[r["id"]] for r in rows]
        run["provider"] = f"predictions:{args.predictions.name}"
    elif args.provider == "baseline":
        preds = [baseline.predict(r["text"]) for r in rows]
        run["provider"] = "baseline"
    else:
        preds, info = asyncio.run(predict_llm(rows, args.provider, args.concurrency))
        run.update(info)
    run["seconds"] = round(time.perf_counter() - started, 2)

    metrics = compute_metrics([r["label"] for r in rows], preds)
    report = {"run": run, "metrics": metrics, "gates": check_gates(metrics, args)}
    if args.out:
        args.out.write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
    md = render_markdown(report)
    if args.markdown:
        args.markdown.write_text(md, encoding="utf-8")
    print(md)
    failed = [g["name"] for g in report["gates"] if not g["passed"]]
    if failed:
        print(f"quality gate failed: {', '.join(failed)}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
