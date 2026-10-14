import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import run_eval  # noqa: E402

DATASET = Path(__file__).resolve().parent / "dataset.jsonl"


def test_dataset_shape_and_privacy():
    import re

    rows = run_eval.load_jsonl(DATASET)
    assert len(rows) == 300 and len({r["id"] for r in rows}) == 300
    assert sum(r["label"] == "legitimate" for r in rows) >= 100
    for r in rows:
        assert r["is_scam"] == (r["label"] != "legitimate")
        assert r["lang"] in {"en", "hi", "kn"}
        # No unmasked 10-digit Indian mobile numbers.
        assert not re.search(r"(?<!\d)[6-9]\d{9}(?!\d)", r["text"]), r["id"]


def test_metrics_math():
    m = run_eval.compute_metrics(["a", "a", "legitimate", "b"], ["a", "b", "legitimate", "b"])
    assert m["per_category"]["a"] == {"precision": 1.0, "recall": 0.5, "f1": 0.6667, "support": 2}
    assert m["per_category"]["b"]["precision"] == 0.5
    assert m["binary"]["scam_recall"] == 1.0 and m["binary"]["legit_false_positive_rate"] == 0.0
    assert m["confusion"] == {"a->b": 1}


def test_baseline_run_and_gate(tmp_path):
    out = tmp_path / "r.json"
    assert run_eval.main(["--out", str(out), "--min-scam-recall", "0.5"]) == 0
    report = json.loads(out.read_text())
    assert report["metrics"]["n"] == 300 and report["gates"][0]["passed"]
    assert run_eval.main(["--min-macro-f1", "1.01"]) == 2


def test_predictions_mode(tmp_path):
    rows = run_eval.load_jsonl(DATASET)
    p = tmp_path / "p.jsonl"
    p.write_text("\n".join(json.dumps({"id": r["id"], "label": r["label"]}) for r in rows))
    out = tmp_path / "r.json"
    assert run_eval.main(["--predictions", str(p), "--out", str(out)]) == 0
    assert json.loads(out.read_text())["metrics"]["accuracy"] == 1.0
