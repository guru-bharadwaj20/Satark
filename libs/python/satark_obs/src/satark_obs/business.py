"""Product-level metrics shared with the Go library (same names and labels).

    satark_scans_total{service,kind,verdict,category,channel}
    satark_llm_calls_total{service,provider,model,outcome}
    satark_llm_call_duration_seconds{service,provider}
    satark_llm_tokens_total{service,provider,direction}
    satark_cache_requests_total{service,cache,result}

Label values outside the allowed sets are recorded as "other" so a caller
bug can never explode cardinality.
"""

from __future__ import annotations

import time
from collections.abc import Iterator
from contextlib import contextmanager

from prometheus_client import CollectorRegistry, Counter, Histogram

VERDICTS = frozenset({"low_risk", "suspicious", "high_risk", "unknown"})
CATEGORIES = frozenset(
    {
        "phishing", "lottery_prize", "kyc_update", "job_offer", "investment", "loan_app",
        "digital_arrest", "sextortion", "courier_customs", "upi_collect", "impersonation",
        "malware_apk", "none",
    }
)
CHANNELS = frozenset({"pwa", "telegram", "api"})
SCAN_KINDS = frozenset({"url", "text", "qr", "image"})
LLM_OUTCOMES = frozenset({"ok", "error", "timeout", "skipped"})
CACHE_RESULTS = frozenset({"hit", "miss", "error"})


def _one_of(value: str, allowed: frozenset[str]) -> str:
    return value if value in allowed else "other"


class BusinessMetrics:
    def __init__(self, service: str, registry: CollectorRegistry) -> None:
        self.service = service
        self.scans = Counter(
            "satark_scans_total", "Completed scans by kind, verdict, scam category and channel.",
            ["service", "kind", "verdict", "category", "channel"], registry=registry,
        )
        self.llm_calls = Counter(
            "satark_llm_calls_total", "LLM calls by provider, model and outcome.",
            ["service", "provider", "model", "outcome"], registry=registry,
        )
        self.llm_duration = Histogram(
            "satark_llm_call_duration_seconds", "LLM call latency in seconds.",
            ["service", "provider"], buckets=(0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30), registry=registry,
        )
        self.llm_tokens = Counter(
            "satark_llm_tokens_total", "LLM tokens consumed, by direction (input/output).",
            ["service", "provider", "direction"], registry=registry,
        )
        self.cache = Counter(
            "satark_cache_requests_total", "Cache lookups by cache name and result (hit/miss/error).",
            ["service", "cache", "result"], registry=registry,
        )

    def scan(self, kind: str, verdict: str, category: str, channel: str) -> None:
        self.scans.labels(
            self.service, _one_of(kind, SCAN_KINDS), _one_of(verdict, VERDICTS),
            _one_of(category, CATEGORIES), _one_of(channel, CHANNELS),
        ).inc()

    def llm_call(
        self, provider: str, model: str, outcome: str, elapsed: float, in_tokens: int = 0, out_tokens: int = 0
    ) -> None:
        self.llm_calls.labels(self.service, provider, model, _one_of(outcome, LLM_OUTCOMES)).inc()
        if outcome != "skipped":
            self.llm_duration.labels(self.service, provider).observe(elapsed)
        if in_tokens > 0:
            self.llm_tokens.labels(self.service, provider, "input").inc(in_tokens)
        if out_tokens > 0:
            self.llm_tokens.labels(self.service, provider, "output").inc(out_tokens)

    @contextmanager
    def time_llm(self, provider: str, model: str) -> Iterator[dict]:
        """Time an LLM call; set ``ctx["in_tokens"]``/``ctx["out_tokens"]`` inside the block.

        Exceptions mark the call as ``error`` (``TimeoutError`` as ``timeout``) and re-raise.
        """
        ctx: dict = {"outcome": "ok", "in_tokens": 0, "out_tokens": 0}
        start = time.perf_counter()
        try:
            yield ctx
        except TimeoutError:
            ctx["outcome"] = "timeout"
            raise
        except Exception:
            ctx["outcome"] = "error"
            raise
        finally:
            self.llm_call(
                provider, model, ctx["outcome"], time.perf_counter() - start, ctx["in_tokens"], ctx["out_tokens"]
            )

    def cache_lookup(self, cache: str, result: str) -> None:
        self.cache.labels(self.service, cache, _one_of(result, CACHE_RESULTS)).inc()
