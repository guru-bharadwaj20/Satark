"""Prometheus metrics for text-analyzer (prefix ``satark_``)."""

from __future__ import annotations

from prometheus_client import Counter, Histogram

REQUESTS = Counter(
    "satark_text_analyzer_requests_total",
    "HTTP requests handled by text-analyzer.",
    ["path", "status"],
)
LATENCY = Histogram(
    "satark_text_analyzer_request_duration_seconds",
    "HTTP request latency.",
    ["path"],
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5),
)
