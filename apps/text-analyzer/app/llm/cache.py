"""Cache LLM classifications by a hash of the *redacted* text.

Only the SHA-256 digest of the redacted text is used as a key; neither raw nor
redacted text is stored. Values are the validated ClassificationResult JSON.
Only successful classifications are cached, so a provider outage never
poisons the cache with fallbacks.
"""

from __future__ import annotations

import hashlib
import logging
import time
from collections import OrderedDict
from dataclasses import replace
from typing import Protocol

from app.llm.classifier import LLMClassifier, LLMOutcome
from app.llm.prompts import PROMPT_VERSION, injection_suspected
from app.llm.schema import ClassificationResult

log = logging.getLogger("satark.text_analyzer.llm.cache")

KEY_PREFIX = "satark:llm:v1:"


def cache_key(
    redacted_text: str,
    *,
    language: str,
    rule_hits: list[str] | None,
    provider: str,
    model: str,
    prompt_version: str = PROMPT_VERSION,
) -> str:
    h = hashlib.sha256()
    for part in (
        prompt_version,
        provider,
        model,
        language,
        ",".join(sorted(set(rule_hits or []))),
        " ".join(redacted_text.split()),  # whitespace-insensitive
    ):
        h.update(part.encode("utf-8"))
        h.update(b"\x1f")
    return KEY_PREFIX + h.hexdigest()


class CacheBackend(Protocol):
    async def get(self, key: str) -> str | None: ...
    async def set(self, key: str, value: str, ttl_s: int) -> None: ...


class MemoryCache:
    """Bounded in-process LRU with TTL; used in tests and as a local fallback."""

    def __init__(self, max_items: int = 2048, clock=time.monotonic) -> None:
        self.max_items, self._clock = max_items, clock
        self._data: OrderedDict[str, tuple[float, str]] = OrderedDict()

    async def get(self, key: str) -> str | None:
        item = self._data.get(key)
        if item is None:
            return None
        expires, value = item
        if self._clock() >= expires:
            self._data.pop(key, None)
            return None
        self._data.move_to_end(key)
        return value

    async def set(self, key: str, value: str, ttl_s: int) -> None:
        self._data[key] = (self._clock() + ttl_s, value)
        self._data.move_to_end(key)
        while len(self._data) > self.max_items:
            self._data.popitem(last=False)


class ValkeyCache:
    """Adapter for a ``redis.asyncio``/``valkey.asyncio`` client."""

    def __init__(self, client) -> None:
        self._client = client

    async def get(self, key: str) -> str | None:
        value = await self._client.get(key)
        if isinstance(value, bytes):
            value = value.decode("utf-8")
        return value

    async def set(self, key: str, value: str, ttl_s: int) -> None:
        await self._client.set(key, value, ex=ttl_s)


class CachedClassifier:
    """Wraps LLMClassifier with a read-through cache. Cache errors are non-fatal."""

    def __init__(self, inner: LLMClassifier, backend: CacheBackend, ttl_s: int = 7 * 86400):
        self.inner, self.backend, self.ttl_s = inner, backend, ttl_s

    def _key(self, text: str, rule_hits, language: str) -> str:
        p = self.inner.provider
        return cache_key(
            text,
            language=language,
            rule_hits=rule_hits,
            provider=getattr(p, "name", "none"),
            model=getattr(p, "model", ""),
        )

    async def classify(
        self, redacted_text: str, rule_hits: list[str] | None = None, language: str = "en"
    ) -> LLMOutcome:
        key = self._key(redacted_text, rule_hits, language)
        try:
            hit = await self.backend.get(key)
        except Exception as exc:  # cache must never break scans
            log.warning("llm cache get failed: %s", type(exc).__name__)
            hit = None
        if hit:
            try:
                result = ClassificationResult.model_validate_json(hit)
                p = self.inner.provider
                return LLMOutcome(
                    result=result,
                    provider=getattr(p, "name", ""),
                    model=getattr(p, "model", ""),
                    cached=True,
                    injection_suspected=injection_suspected(redacted_text),
                )
            except ValueError:
                log.info("discarding unreadable cache entry")

        outcome = await self.inner.classify(redacted_text, rule_hits, language)
        if outcome.result is not None:
            try:
                await self.backend.set(key, outcome.result.model_dump_json(), self.ttl_s)
            except Exception as exc:
                log.warning("llm cache set failed: %s", type(exc).__name__)
        return replace(outcome, cached=False)
