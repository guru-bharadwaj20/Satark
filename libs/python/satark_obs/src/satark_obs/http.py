"""RED (rate, errors, duration) metrics for Starlette/FastAPI services.

Series match the Go library (libs/go/observability) exactly, so one set of
dashboards and SLO rules covers every service:

    satark_http_requests_total{service,method,route,code}
    satark_http_request_duration_seconds{service,method,route}
    satark_http_requests_in_flight{service}
"""

from __future__ import annotations

import time
from collections.abc import Awaitable, Callable, Mapping

from prometheus_client import (
    CONTENT_TYPE_LATEST,
    CollectorRegistry,
    Counter,
    Gauge,
    Histogram,
    generate_latest,
)
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Match
from starlette.types import ASGIApp, Message, Receive, Scope, Send

DEFAULT_BUCKETS = (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.8, 1.0, 2.0, 5.0, 10.0)
UNMATCHED_ROUTE = "unmatched"


class HTTPMetrics:
    """RED collectors for one service, on a dedicated registry."""

    def __init__(self, service: str, registry: CollectorRegistry | None = None) -> None:
        self.service = service
        self.registry = registry or CollectorRegistry(auto_describe=True)
        self.requests = Counter(
            "satark_http_requests_total",
            "HTTP requests handled, by method, route template and status code.",
            ["service", "method", "route", "code"],
            registry=self.registry,
        )
        self.duration = Histogram(
            "satark_http_request_duration_seconds",
            "HTTP request latency in seconds, by method and route template.",
            ["service", "method", "route"],
            buckets=DEFAULT_BUCKETS,
            registry=self.registry,
        )
        self.in_flight = Gauge(
            "satark_http_requests_in_flight",
            "HTTP requests currently being served.",
            ["service"],
            registry=self.registry,
        )

    def observe(self, method: str, route: str, status: int, elapsed: float) -> None:
        self.requests.labels(self.service, method, route, str(status)).inc()
        self.duration.labels(self.service, method, route).observe(elapsed)

    def render(self) -> Response:
        return Response(generate_latest(self.registry), media_type=CONTENT_TYPE_LATEST)


def _route_template(scope: Scope) -> str:
    """Resolve the matched route template, never the raw path (cardinality)."""
    app = scope.get("app")
    router = getattr(app, "router", None)
    for route in getattr(router, "routes", []):
        match, _ = route.matches(scope)
        if match == Match.FULL:
            return getattr(route, "path", UNMATCHED_ROUTE)
    return UNMATCHED_ROUTE


class PrometheusMiddleware:
    """Pure ASGI middleware (no BaseHTTPMiddleware overhead, streaming-safe)."""

    def __init__(self, app: ASGIApp, metrics: HTTPMetrics, skip: tuple[str, ...] = ("/metrics",)) -> None:
        self.app = app
        self.metrics = metrics
        self.skip = skip

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http" or scope.get("path") in self.skip:
            await self.app(scope, receive, send)
            return

        status = 500
        start = time.perf_counter()
        gauge = self.metrics.in_flight.labels(self.metrics.service)
        gauge.inc()

        async def send_wrapper(message: Message) -> None:
            nonlocal status
            if message["type"] == "http.response.start":
                status = message["status"]
            await send(message)

        try:
            await self.app(scope, receive, send_wrapper)
        finally:
            gauge.dec()
            self.metrics.observe(scope["method"], _route_template(scope), status, time.perf_counter() - start)


ReadinessCheck = Callable[[], Awaitable[None]]


def mount_endpoints(app, metrics: HTTPMetrics, checks: Mapping[str, ReadinessCheck] | None = None) -> None:
    """Add /healthz, /readyz and /metrics to a Starlette or FastAPI app."""
    checks = dict(checks or {})

    async def healthz(_: Request) -> Response:
        return JSONResponse({"status": "ok"})

    async def readyz(_: Request) -> Response:
        failed: dict[str, str] = {}
        for name, check in checks.items():
            try:
                await check()
            except Exception as exc:  # noqa: BLE001 - report any dependency failure
                failed[name] = type(exc).__name__
        if failed:
            return JSONResponse({"status": "unready", "failed": failed}, status_code=503)
        return JSONResponse({"status": "ready"})

    async def metrics_endpoint(_: Request) -> Response:
        return metrics.render()

    app.add_route("/healthz", healthz, methods=["GET"])
    app.add_route("/readyz", readyz, methods=["GET"])
    app.add_route("/metrics", metrics_endpoint, methods=["GET"])
