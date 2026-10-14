"""Text embeddings via Ollama's /api/embed (local; runbooks never leave the cluster)."""

from __future__ import annotations

import httpx


class EmbeddingError(Exception):
    pass


class OllamaEmbedder:
    def __init__(
        self,
        base_url: str,
        model: str = "nomic-embed-text",
        dim: int = 768,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.base_url, self.model, self.dim = base_url.rstrip("/"), model, dim
        self._client = client or httpx.AsyncClient(timeout=60.0)

    async def embed(self, texts: list[str]) -> list[list[float]]:
        if not texts:
            return []
        try:
            r = await self._client.post(
                f"{self.base_url}/api/embed", json={"model": self.model, "input": texts}
            )
            r.raise_for_status()
            vectors = r.json()["embeddings"]
        except (httpx.HTTPError, KeyError, ValueError) as exc:
            raise EmbeddingError(f"embedding failed: {type(exc).__name__}") from exc
        if len(vectors) != len(texts) or any(len(v) != self.dim for v in vectors):
            raise EmbeddingError(f"expected {len(texts)} vectors of dim {self.dim}")
        return vectors
