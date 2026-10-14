import asyncio
import json

from llm_fakes import FakeProvider
from prometheus_client import REGISTRY

from app.llm.cache import CachedClassifier, MemoryCache
from app.llm.classifier import LLMClassifier
from app.llm.metrics import estimate_cost, load_prices
from app.llm.provider import LLMError

GOOD = json.dumps(
    {
        "category": "otp_pin",
        "is_scam": True,
        "confidence": 0.8,
        "reasons": [],
        "explanation": "Never share an OTP.",
    }
)
LBL = {"provider": "fake", "model": "fake-model"}


def val(name, **labels):
    return REGISTRY.get_sample_value(name, labels) or 0.0


def test_tokens_outcomes_and_cache_hits_counted():
    before_ok = val("satark_llm_requests_total", **LBL, outcome="ok")
    before_hit = val("satark_llm_requests_total", **LBL, outcome="cached")
    before_tok = val("satark_llm_tokens_total", **LBL, direction="input")
    c = CachedClassifier(LLMClassifier(FakeProvider([GOOD])), MemoryCache())
    asyncio.run(c.classify("metrics-test"))
    asyncio.run(c.classify("metrics-test"))
    assert val("satark_llm_requests_total", **LBL, outcome="ok") == before_ok + 1
    assert val("satark_llm_requests_total", **LBL, outcome="cached") == before_hit + 1
    assert val("satark_llm_tokens_total", **LBL, direction="input") == before_tok + 100


def test_fallback_counted():
    before = val("satark_llm_requests_total", **LBL, outcome="provider_error")
    asyncio.run(LLMClassifier(FakeProvider(error=LLMError("x"))).classify("y"))
    assert val("satark_llm_requests_total", **LBL, outcome="provider_error") == before + 1


def test_cost_estimate_and_override(monkeypatch):
    assert estimate_cost("unknown-local-model", 10_000, 10_000) == 0.0
    assert estimate_cost("claude-haiku-4-5", 1_000_000, 0) == 1.0
    monkeypatch.setenv("SATARK_LLM_PRICES", '{"m": [2, 4]}')
    assert load_prices()["m"] == (2.0, 4.0)
    monkeypatch.setenv("SATARK_LLM_PRICES", "not json")
    assert "m" not in load_prices()
