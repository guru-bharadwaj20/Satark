import asyncio
import json

import httpx
import pytest
from rag_fakes import FakeEmbedder, FakeStore

from app.rag import chunk as chunk_mod
from app.rag.chunk import chunk_markdown
from app.rag.embed import EmbeddingError, OllamaEmbedder
from app.rag.ingest import ingest
from app.rag.store import schema_sql, to_pgvector

RB = """# RB-001: CrashLoopBackOff and ImagePullBackOff

## Symptoms
Pods restart repeatedly.

## Diagnosis
### Check events
```bash
# not a heading
kubectl -n satark describe pod <pod>
```
### Check logs
kubectl logs --previous

## Fix
Roll back the deployment.
"""


def test_chunks_follow_headings_and_ignore_code_comments():
    chunks = chunk_markdown("RB-001-crashloop-imagepull", RB)
    assert [c.heading for c in chunks] == [
        "Symptoms",
        "Diagnosis > Check events",
        "Diagnosis > Check logs",
        "Fix",
    ]
    assert all(c.title.startswith("RB-001") for c in chunks)
    assert "# not a heading" in chunks[1].content
    assert chunks[0].id == "RB-001-crashloop-imagepull#0"


def test_long_sections_split_with_overlap(monkeypatch):
    monkeypatch.setattr(chunk_mod, "MAX_CHARS", 100)
    monkeypatch.setattr(chunk_mod, "OVERLAP", 20)
    body = "\n\n".join(f"paragraph {i} " + "x" * 30 for i in range(10))
    chunks = chunk_markdown("d", "# T\n## S\n" + body)
    assert len(chunks) > 3 and all(len(c.content) <= 100 for c in chunks)


def test_ingest_is_idempotent_and_removes_stale():
    store, emb = FakeStore(), FakeEmbedder()
    chunks = chunk_markdown("RB-001", RB)
    s1 = asyncio.run(ingest(chunks, emb, store))
    assert s1.embedded == 4 and s1.unchanged == 0
    s2 = asyncio.run(ingest(chunks, emb, store))
    assert s2.embedded == 0 and s2.unchanged == 4 and emb.calls == 4
    s3 = asyncio.run(ingest(chunks[:2], emb, store))
    assert s3.deleted == 2 and len(store.rows) == 2


def test_schema_and_vector_literal():
    sql = " ".join(schema_sql(768))
    assert "vector(768)" in sql and "hnsw" in sql and "vector_cosine_ops" in sql
    assert to_pgvector([0.5, 1.0]) == "[0.500000,1.000000]"


def test_ollama_embedder():
    def handler(request):
        body = json.loads(request.content)
        return httpx.Response(200, json={"embeddings": [[0.1, 0.2]] * len(body["input"])})

    e = OllamaEmbedder(
        "http://o", dim=2, client=httpx.AsyncClient(transport=httpx.MockTransport(handler))
    )
    assert asyncio.run(e.embed(["a", "b"])) == [[0.1, 0.2], [0.1, 0.2]]
    bad = OllamaEmbedder(
        "http://o", dim=3, client=httpx.AsyncClient(transport=httpx.MockTransport(handler))
    )
    with pytest.raises(EmbeddingError):
        asyncio.run(bad.embed(["a"]))
