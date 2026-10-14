import asyncio
import json

from llm_fakes import FakeProvider

from app.llm.classifier import CircuitBreaker, FallbackReason, LLMClassifier
from app.llm.provider import LLMError

GOOD = json.dumps(
    {
        "category": "otp_pin",
        "is_scam": True,
        "confidence": 0.8,
        "reasons": ["Asks for the OTP"],
        "explanation": "Never share an OTP.",
        "language": "en",
    }
)


def run(coro):
    return asyncio.run(coro)


def test_success():
    out = run(LLMClassifier(FakeProvider([GOOD])).classify("share otp", ["otp"]))
    assert not out.fallback and out.result.category.value == "otp_pin"
    assert out.input_tokens == 100


def test_timeout_falls_back():
    clf = LLMClassifier(FakeProvider([GOOD], delay=1.0), timeout_s=0.01)
    out = run(clf.classify("x"))
    assert out.fallback and out.fallback_reason is FallbackReason.TIMEOUT


def test_provider_error_falls_back():
    out = run(LLMClassifier(FakeProvider(error=LLMError("503"))).classify("x"))
    assert out.fallback_reason is FallbackReason.PROVIDER_ERROR


def test_invalid_then_valid_retries_once():
    p = FakeProvider(["not json", GOOD])
    out = run(LLMClassifier(p, retry_invalid=1).classify("x"))
    assert not out.fallback and len(p.calls) == 2 and out.input_tokens == 200


def test_invalid_twice_falls_back():
    out = run(LLMClassifier(FakeProvider(["nope"]), retry_invalid=1).classify("x"))
    assert out.fallback_reason is FallbackReason.INVALID_OUTPUT


def test_disabled_when_no_provider():
    assert run(LLMClassifier(None).classify("x")).fallback_reason is FallbackReason.DISABLED


def test_circuit_breaker_opens_and_recovers():
    now = [0.0]
    breaker = CircuitBreaker(threshold=2, cooldown_s=30, clock=lambda: now[0])
    p = FakeProvider(error=LLMError("down"))
    clf = LLMClassifier(p, breaker=breaker)
    run(clf.classify("x"))
    run(clf.classify("x"))
    assert run(clf.classify("x")).fallback_reason is FallbackReason.CIRCUIT_OPEN
    assert len(p.calls) == 2
    now[0] = 31
    p.error, p.replies = None, [GOOD]
    assert not run(clf.classify("x")).fallback


def test_requested_language_is_passed_to_prompt():
    p = FakeProvider([GOOD])
    run(LLMClassifier(p).classify("x", language="kn"))
    assert "Kannada" in p.calls[0].messages[0].content
