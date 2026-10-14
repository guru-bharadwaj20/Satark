"""Retrieve the most relevant runbook sections for an alert."""

from __future__ import annotations

import re

from app.alerts import Alert
from app.rag.store import Hit

_REASONS = re.compile(
    r"\b(CrashLoopBackOff|ImagePullBackOff|ErrImagePull|OOMKilled|Evicted|FailedScheduling|"
    r"FailedMount|Unhealthy|BackOff|NodeNotReady|DiskPressure|MemoryPressure|"
    r"x509|certificate|connection refused|no such host|timeout|deadline exceeded)\b",
    re.I,
)
_RB_ID = re.compile(r"\b(RB-\d{3})\b")


def build_query(alert: Alert, context_text: str = "") -> str:
    # "SatarkLLMBudgetExceeded" -> "Satark LLM Budget Exceeded" so embeddings see words.
    words = re.sub(r"(?<=[a-z0-9])(?=[A-Z])|(?<=[A-Z])(?=[A-Z][a-z])", " ", alert.name)
    parts = [
        words,
        alert.annotations.get("summary", ""),
        alert.annotations.get("description", ""),
    ]
    reasons = sorted({m.group(1) for m in _REASONS.finditer(context_text)})
    if reasons:
        parts.append("signals: " + ", ".join(reasons))
    return "\n".join(p for p in parts if p)[:2000]


def linked_runbook(alert: Alert) -> str | None:
    m = _RB_ID.search(
        alert.annotations.get("runbook_url", "") + " " + alert.annotations.get("runbook", "")
    )
    return m.group(1) if m else None


class Retriever:
    def __init__(self, embedder, store, k: int = 4, min_score: float = 0.3, boost: float = 0.15):
        self.embedder, self.store = embedder, store
        self.k, self.min_score, self.boost = k, min_score, boost

    async def retrieve(self, query: str, prefer_doc: str | None = None) -> list[Hit]:
        [vec] = await self.embedder.embed([query])
        hits = await self.store.search(vec, k=self.k * 3)
        rescored = []
        for h in hits:
            score = h.score + (self.boost if prefer_doc and h.doc.startswith(prefer_doc) else 0.0)
            rescored.append(Hit(h.id, h.doc, h.title, h.heading, h.content, round(score, 4)))
        rescored = [h for h in rescored if h.score >= self.min_score]
        rescored.sort(key=lambda h: -h.score)
        # At most two sections per runbook so one document does not crowd out others.
        out, per_doc = [], {}
        for h in rescored:
            if per_doc.get(h.doc, 0) >= 2:
                continue
            per_doc[h.doc] = per_doc.get(h.doc, 0) + 1
            out.append(h)
            if len(out) == self.k:
                break
        return out

    async def for_alert(self, alert: Alert, context_text: str = "") -> list[Hit]:
        return await self.retrieve(build_query(alert, context_text), linked_runbook(alert))


def format_sources(hits: list[Hit], max_chars: int = 1200) -> str:
    if not hits:
        return "No matching runbook sections were found."
    blocks = []
    for i, h in enumerate(hits, 1):
        body = h.content if len(h.content) <= max_chars else h.content[:max_chars] + " ..."
        blocks.append(f"[{i}] {h.title} - {h.heading} (score {h.score:.2f})\n{body}")
    return "\n\n".join(blocks)
