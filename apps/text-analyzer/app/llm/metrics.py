"""Prometheus metrics for LLM usage, tokens and estimated cost.

``prometheus_client`` is optional; without it every recorder is a no-op.
Prices are USD per million tokens and can be overridden with
``SATARK_LLM_PRICES`` (JSON: {"model": [input_price, output_price]}).
Local models (Ollama) cost 0 but their tokens are still counted.
"""

from __future__ import annotations

import json
import logging
import os

log = logging.getLogger("satark.text_analyzer.llm.metrics")

DEFAULT_PRICES: dict[str, tuple[float, float]] = {
    "anthropic.claude-3-haiku-20240307-v1:0": (0.25, 1.25),
    "claude-haiku-4-5": (1.0, 5.0),
}

try:
    from prometheus_client import Counter, Histogram

    LLM_REQUESTS = Counter(
        "satark_llm_requests_total",
        "LLM classification attempts by outcome (ok, cached or a fallback reason).",
        ["provider", "model", "outcome"],
    )
    LLM_TOKENS = Counter(
        "satark_llm_tokens_total",
        "LLM tokens consumed.",
        ["provider", "model", "direction"],
    )
    LLM_COST = Counter(
        "satark_llm_estimated_cost_usd_total",
        "Estimated LLM spend in USD from the configured price table.",
        ["provider", "model"],
    )
    LLM_LATENCY = Histogram(
        "satark_llm_request_duration_seconds",
        "Latency of successful LLM calls.",
        ["provider", "model"],
        buckets=(0.25, 0.5, 1, 2, 3, 5, 8, 13),
    )
    ENABLED = True
except ModuleNotFoundError:  # pragma: no cover - depends on environment
    ENABLED = False


def load_prices() -> dict[str, tuple[float, float]]:
    prices = dict(DEFAULT_PRICES)
    raw = os.environ.get("SATARK_LLM_PRICES")
    if raw:
        try:
            prices.update({k: (float(v[0]), float(v[1])) for k, v in json.loads(raw).items()})
        except (ValueError, TypeError, IndexError, AttributeError):
            log.warning("ignoring invalid SATARK_LLM_PRICES")
    return prices


_PRICES = load_prices()


def estimate_cost(model: str, input_tokens: int, output_tokens: int) -> float:
    pin, pout = _PRICES.get(model, (0.0, 0.0))
    return (input_tokens * pin + output_tokens * pout) / 1_000_000


def record_outcome(outcome) -> None:
    """Record one LLMOutcome (from app.llm.classifier)."""
    if not ENABLED:
        return
    provider = outcome.provider or "none"
    model = outcome.model or "none"
    if outcome.cached:
        label = "cached"
    elif outcome.result is not None:
        label = "ok"
    else:
        label = outcome.fallback_reason.value
    LLM_REQUESTS.labels(provider, model, label).inc()
    if outcome.input_tokens:
        LLM_TOKENS.labels(provider, model, "input").inc(outcome.input_tokens)
    if outcome.output_tokens:
        LLM_TOKENS.labels(provider, model, "output").inc(outcome.output_tokens)
    cost = estimate_cost(model, outcome.input_tokens, outcome.output_tokens)
    if cost:
        LLM_COST.labels(provider, model).inc(cost)
    if label == "ok" and outcome.latency_s:
        LLM_LATENCY.labels(provider, model).observe(outcome.latency_s)
