import asyncio
import json

from llm_fakes import FakeProvider

from app.llm.cache import CachedClassifier, MemoryCache, ValkeyCache, cache_key
from app.llm.classifier import LLMClassifier
from app.llm.provider import LLMError

GOOD = json.dumps(
    {
        "category": "kyc_banking",
        "is_scam": True,
        "confidence": 0.9,
        "reasons": ["KYC threat"],
        "explanation": "Do not click the link.",
        "language": "en",
    }
)
KW = dict(language="en", rule_hits=["kyc"], provider="p", model="m")


def run(c):
    return asyncio.run(c)


def test_key_is_hash_without_text_and_stable():
    k = cache_key("Update KYC at [URL]", **KW)
    assert "KYC" not in k and len(k.split(":")[-1]) == 64
    assert k == cache_key("Update  KYC at\n[URL]", **KW)
    assert k != cache_key("Update KYC at [URL]", **{**KW, "language": "hi"})
    assert k != cache_key("Update KYC at [URL]", **{**KW, "model": "m2"})
    assert k != cache_key("Update KYC at [URL]", **KW, prompt_version="classify-v2")


def test_second_call_is_served_from_cache():
    p = FakeProvider([GOOD])
    c = CachedClassifier(LLMClassifier(p), MemoryCache())
    first = run(c.classify("Update KYC", ["kyc"]))
    second = run(c.classify("Update KYC", ["kyc"]))
    assert not first.cached and second.cached
    assert second.result == first.result and second.input_tokens == 0
    assert len(p.calls) == 1


def test_fallbacks_are_not_cached():
    p = FakeProvider(error=LLMError("down"))
    backend = MemoryCache()
    c = CachedClassifier(LLMClassifier(p), backend)
    assert run(c.classify("x")).fallback
    assert backend._data == {}


def test_ttl_and_lru():
    now = [0.0]
    m = MemoryCache(max_items=2, clock=lambda: now[0])
    run(m.set("a", "1", 10))
    run(m.set("b", "2", 10))
    run(m.set("c", "3", 10))
    assert run(m.get("a")) is None and run(m.get("c")) == "3"
    now[0] = 11
    assert run(m.get("c")) is None


class BrokenBackend:
    async def get(self, key):
        raise ConnectionError

    async def set(self, key, value, ttl_s):
        raise ConnectionError


def test_cache_outage_is_non_fatal():
    c = CachedClassifier(LLMClassifier(FakeProvider([GOOD])), BrokenBackend())
    assert not run(c.classify("x")).fallback


class FakeRedis:
    def __init__(self):
        self.d, self.ex = {}, {}

    async def get(self, k):
        return self.d.get(k)

    async def set(self, k, v, ex=None):
        self.d[k] = v.encode()
        self.ex[k] = ex


def test_valkey_adapter():
    r = FakeRedis()
    v = ValkeyCache(r)
    run(v.set("k", "val", 60))
    assert run(v.get("k")) == "val" and r.ex["k"] == 60
