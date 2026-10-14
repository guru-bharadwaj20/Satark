"""FastAPI application for text-analyzer."""

from __future__ import annotations

import logging
import time
from collections.abc import Awaitable, Callable
from contextlib import asynccontextmanager
from typing import Literal

from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.responses import JSONResponse
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from pydantic import BaseModel, Field

from app import __version__
from app.config import load_settings
from app.metrics import LATENCY, REDACTIONS, REQUESTS, RULE_HITS
from app.redact import redact
from app.rules import RuleEngine, RuleError

settings = load_settings()
logging.basicConfig(
    level=settings.log_level.upper(),
    format='{"ts":"%(asctime)s","level":"%(levelname)s","logger":"%(name)s","msg":"%(message)s"}',
)
log = logging.getLogger("text-analyzer")


class State:
    engine: RuleEngine | None = None


state = State()


def load_engine() -> None:
    try:
        state.engine = RuleEngine.from_dir(settings.rules_dir)
    except (OSError, RuleError) as exc:
        log.error("failed to load rules: %s", exc)
        state.engine = None


@asynccontextmanager
async def lifespan(_: FastAPI):
    load_engine()
    yield


app = FastAPI(title="satark-text-analyzer", version=__version__, lifespan=lifespan)

_KNOWN_PATHS = {"/healthz", "/readyz", "/metrics", "/v1/analyze"}


@app.middleware("http")
async def observe(request: Request, call_next: Callable[[Request], Awaitable[Response]]):
    path = request.url.path if request.url.path in _KNOWN_PATHS else "other"
    start = time.perf_counter()
    response = await call_next(request)
    LATENCY.labels(path).observe(time.perf_counter() - start)
    REQUESTS.labels(path, str(response.status_code)).inc()
    return response


class AnalyzeRequest(BaseModel):
    text: str = Field(min_length=1)
    lang: Literal["en", "hi", "kn"] = "en"


class RuleMatch(BaseModel):
    id: str
    category: str
    weight: int
    reason: str


class AnalyzeResponse(BaseModel):
    redacted_text: str
    redactions: dict[str, int]
    matches: list[RuleMatch]
    lang: str


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/readyz")
def readyz() -> JSONResponse:
    if state.engine is None or len(state.engine) == 0:
        return JSONResponse({"status": "degraded"}, status_code=503)
    return JSONResponse({"status": "ok"})


@app.get("/metrics")
def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/v1/analyze", response_model=AnalyzeResponse)
def analyze(req: AnalyzeRequest) -> AnalyzeResponse:
    if len(req.text) > settings.max_text_chars:
        raise HTTPException(status_code=413, detail="text too long")
    if state.engine is None:
        raise HTTPException(status_code=503, detail="rules not loaded")
    result = redact(req.text)
    for kind, n in result.counts.items():
        REDACTIONS.labels(kind).inc(n)
    matches = state.engine.evaluate(result.text, req.lang)
    for m in matches:
        RULE_HITS.labels(m.category).inc()
    return AnalyzeResponse(
        redacted_text=result.text,
        redactions=dict(result.counts),
        matches=[RuleMatch(**m.as_dict()) for m in matches],
        lang=req.lang,
    )
