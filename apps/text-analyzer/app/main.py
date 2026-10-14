"""FastAPI application for text-analyzer."""

from __future__ import annotations

import logging
import time
from collections.abc import Awaitable, Callable

from fastapi import FastAPI, Request, Response
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from app import __version__
from app.config import load_settings
from app.metrics import LATENCY, REQUESTS

settings = load_settings()
logging.basicConfig(
    level=settings.log_level.upper(),
    format='{"ts":"%(asctime)s","level":"%(levelname)s","logger":"%(name)s","msg":"%(message)s"}',
)
log = logging.getLogger("text-analyzer")

app = FastAPI(title="satark-text-analyzer", version=__version__)

_KNOWN_PATHS = {"/healthz", "/readyz", "/metrics", "/v1/analyze"}


@app.middleware("http")
async def observe(request: Request, call_next: Callable[[Request], Awaitable[Response]]):
    path = request.url.path if request.url.path in _KNOWN_PATHS else "other"
    start = time.perf_counter()
    response = await call_next(request)
    LATENCY.labels(path).observe(time.perf_counter() - start)
    REQUESTS.labels(path, str(response.status_code)).inc()
    return response


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/readyz")
def readyz() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/metrics")
def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)
