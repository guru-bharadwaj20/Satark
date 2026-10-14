"""Embed runbooks into pgvector. Idempotent: unchanged chunks are skipped.

    python -m app.rag.ingest --dir docs/runbooks
Reads OPSBOT_DATABASE_URL, OPSBOT_LLM_URL (Ollama), OPSBOT_EMBED_MODEL.
"""

from __future__ import annotations

import argparse
import asyncio
import logging
from dataclasses import dataclass

from app.rag.chunk import Chunk, load_runbooks

log = logging.getLogger("satark.opsbot.ingest")
BATCH = 16


@dataclass
class IngestStats:
    total: int = 0
    embedded: int = 0
    unchanged: int = 0
    deleted: int = 0


async def ingest(chunks: list[Chunk], embedder, store) -> IngestStats:
    stats = IngestStats(total=len(chunks))
    await store.migrate()
    existing = await store.existing()
    todo = [c for c in chunks if existing.get(c.id) != c.sha]
    stats.unchanged = len(chunks) - len(todo)
    for i in range(0, len(todo), BATCH):
        batch = todo[i : i + BATCH]
        vectors = await embedder.embed([c.embedding_text() for c in batch])
        await store.upsert(batch, vectors)
        stats.embedded += len(batch)
    stale = sorted(set(existing) - {c.id for c in chunks})
    await store.delete(stale)
    stats.deleted = len(stale)
    return stats


async def _main(directory: str) -> None:
    from app.config import get_settings
    from app.rag.embed import OllamaEmbedder
    from app.rag.store import PgVectorStore

    s = get_settings()
    store = await PgVectorStore.connect(s.database_url.get_secret_value(), s.embed_dim)
    embedder = OllamaEmbedder(s.llm_url, s.embed_model, s.embed_dim)
    stats = await ingest(load_runbooks(directory), embedder, store)
    log.info("ingest done: %s", stats)
    print(stats)


if __name__ == "__main__":  # pragma: no cover
    logging.basicConfig(level=logging.INFO)
    ap = argparse.ArgumentParser(description="Embed runbooks into pgvector")
    ap.add_argument("--dir", default="/app/docs/runbooks")
    asyncio.run(_main(ap.parse_args().dir))
