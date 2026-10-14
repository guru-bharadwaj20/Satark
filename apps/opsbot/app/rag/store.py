"""pgvector storage for runbook chunks (on the CNPG cluster satark-pg)."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from app.rag.chunk import Chunk


def schema_sql(dim: int) -> list[str]:
    return [
        "CREATE EXTENSION IF NOT EXISTS vector",
        f"""CREATE TABLE IF NOT EXISTS opsbot_runbook_chunks (
            id          text PRIMARY KEY,
            doc         text NOT NULL,
            title       text NOT NULL,
            heading     text NOT NULL,
            ordinal     int  NOT NULL,
            content     text NOT NULL,
            content_sha text NOT NULL,
            embedding   vector({dim}) NOT NULL,
            updated_at  timestamptz NOT NULL DEFAULT now()
        )""",
        "CREATE INDEX IF NOT EXISTS opsbot_runbook_chunks_embedding_idx "
        "ON opsbot_runbook_chunks USING hnsw (embedding vector_cosine_ops)",
        "CREATE INDEX IF NOT EXISTS opsbot_runbook_chunks_doc_idx ON opsbot_runbook_chunks (doc)",
    ]


UPSERT_SQL = """
INSERT INTO opsbot_runbook_chunks
  (id, doc, title, heading, ordinal, content, content_sha, embedding)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s::vector)
ON CONFLICT (id) DO UPDATE SET
  doc = EXCLUDED.doc, title = EXCLUDED.title, heading = EXCLUDED.heading,
  ordinal = EXCLUDED.ordinal, content = EXCLUDED.content,
  content_sha = EXCLUDED.content_sha, embedding = EXCLUDED.embedding, updated_at = now()
"""

SEARCH_SQL = """
SELECT id, doc, title, heading, content, 1 - (embedding <=> %s::vector) AS score
FROM opsbot_runbook_chunks
ORDER BY embedding <=> %s::vector
LIMIT %s
"""


def to_pgvector(vec: list[float]) -> str:
    return "[" + ",".join(f"{x:.6f}" for x in vec) + "]"


@dataclass(frozen=True)
class Hit:
    id: str
    doc: str
    title: str
    heading: str
    content: str
    score: float


class PgVectorStore:
    """Thin async wrapper around a psycopg AsyncConnection."""

    def __init__(self, conn: Any, dim: int = 768) -> None:
        self.conn, self.dim = conn, dim

    @classmethod
    async def connect(cls, dsn: str, dim: int = 768) -> PgVectorStore:
        import psycopg  # optional dependency (extra "rag")

        return cls(await psycopg.AsyncConnection.connect(dsn, autocommit=True), dim)

    async def migrate(self) -> None:
        async with self.conn.cursor() as cur:
            for stmt in schema_sql(self.dim):
                await cur.execute(stmt)

    async def existing(self) -> dict[str, str]:
        async with self.conn.cursor() as cur:
            await cur.execute("SELECT id, content_sha FROM opsbot_runbook_chunks")
            return {r[0]: r[1] for r in await cur.fetchall()}

    async def upsert(self, chunks: list[Chunk], vectors: list[list[float]]) -> None:
        async with self.conn.cursor() as cur:
            for c, v in zip(chunks, vectors, strict=True):
                await cur.execute(
                    UPSERT_SQL,
                    (c.id, c.doc, c.title, c.heading, c.ordinal, c.content, c.sha, to_pgvector(v)),
                )

    async def delete(self, ids: list[str]) -> None:
        if ids:
            async with self.conn.cursor() as cur:
                await cur.execute("DELETE FROM opsbot_runbook_chunks WHERE id = ANY(%s)", (ids,))

    async def search(self, vector: list[float], k: int = 4) -> list[Hit]:
        v = to_pgvector(vector)
        async with self.conn.cursor() as cur:
            await cur.execute(SEARCH_SQL, (v, v, k))
            return [Hit(*row[:5], float(row[5])) for row in await cur.fetchall()]
