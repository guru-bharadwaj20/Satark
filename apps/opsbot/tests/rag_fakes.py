"""In-memory stand-ins for the embedder and pgvector store."""

from __future__ import annotations

import math
import re

from app.rag.store import Hit

VOCAB = [
    "crashloop",
    "image",
    "pull",
    "postgres",
    "certificate",
    "disk",
    "dns",
    "tunnel",
    "budget",
    "llm",
    "rollback",
    "oom",
    "backoff",
    "token",
]


def bag(text: str) -> list[float]:
    words = re.findall(r"[a-z]+", text.lower())
    v = [float(sum(w.startswith(t) for w in words)) for t in VOCAB]
    n = math.sqrt(sum(x * x for x in v)) or 1.0
    return [x / n for x in v]


class FakeEmbedder:
    dim = len(VOCAB)

    def __init__(self):
        self.calls = 0

    async def embed(self, texts):
        self.calls += len(texts)
        return [bag(t) for t in texts]


class FakeStore:
    def __init__(self):
        self.rows: dict[str, tuple] = {}

    async def migrate(self):
        pass

    async def existing(self):
        return {k: v[0].sha for k, v in self.rows.items()}

    async def upsert(self, chunks, vectors):
        for c, v in zip(chunks, vectors, strict=True):
            self.rows[c.id] = (c, v)

    async def delete(self, ids):
        for i in ids:
            self.rows.pop(i, None)

    async def search(self, vector, k=4):
        scored = []
        for c, v in self.rows.values():
            s = sum(a * b for a, b in zip(vector, v, strict=True))
            scored.append(Hit(c.id, c.doc, c.title, c.heading, c.content, s))
        return sorted(scored, key=lambda h: -h.score)[:k]
