"""FastAPI application for qr-decoder."""

from __future__ import annotations

import logging
import os
import time
from collections.abc import Awaitable, Callable

from fastapi import FastAPI, File, HTTPException, Request, Response, UploadFile
from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest
from pydantic import BaseModel

from app import __version__
from app.decode import DecodeError, decode_image
from app.upi import UPIParseError, parse_upi

MAX_UPLOAD_BYTES = int(os.getenv("MAX_UPLOAD_BYTES", str(5 * 1024 * 1024)))

logging.basicConfig(
    level=os.getenv("LOG_LEVEL", "info").upper(),
    format='{"ts":"%(asctime)s","level":"%(levelname)s","logger":"%(name)s","msg":"%(message)s"}',
)
log = logging.getLogger("qr-decoder")

REQUESTS = Counter(
    "satark_qr_decoder_requests_total", "HTTP requests handled by qr-decoder.", ["path", "status"]
)
LATENCY = Histogram("satark_qr_decoder_request_duration_seconds", "HTTP request latency.", ["path"])
CODES = Counter("satark_qr_decoder_codes_total", "Decoded QR codes, by kind.", ["kind"])

app = FastAPI(title="satark-qr-decoder", version=__version__)
_KNOWN_PATHS = {"/healthz", "/readyz", "/metrics", "/v1/decode"}


@app.middleware("http")
async def observe(request: Request, call_next: Callable[[Request], Awaitable[Response]]):
    path = request.url.path if request.url.path in _KNOWN_PATHS else "other"
    start = time.perf_counter()
    response = await call_next(request)
    LATENCY.labels(path).observe(time.perf_counter() - start)
    REQUESTS.labels(path, str(response.status_code)).inc()
    return response


class UPIOut(BaseModel):
    payee_address: str | None = None
    payee_name: str | None = None
    amount: str | None = None
    currency: str | None = None
    note: str | None = None
    action: str
    direction: str
    issues: list[str]
    summary: str


class DecodedCode(BaseModel):
    format: str
    kind: str
    text: str | None = None
    upi: UPIOut | None = None


class DecodeResponse(BaseModel):
    codes: list[DecodedCode]


def classify(text: str) -> str:
    lower = text.lower()
    if lower.startswith("upi://"):
        return "upi"
    if lower.startswith(("http://", "https://")):
        return "url"
    return "text"


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/readyz")
def readyz() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/metrics")
def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/v1/decode", response_model=DecodeResponse)
async def decode(image: UploadFile = File(...)) -> DecodeResponse:  # noqa: B008
    data = await image.read(MAX_UPLOAD_BYTES + 1)
    if len(data) > MAX_UPLOAD_BYTES:
        raise HTTPException(status_code=413, detail="image too large")
    if not data:
        raise HTTPException(status_code=400, detail="empty upload")
    try:
        codes = decode_image(data)
    except DecodeError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    out: list[DecodedCode] = []
    for c in codes:
        kind = classify(c.text)
        if kind == "upi":
            try:
                intent = parse_upi(c.text)
            except UPIParseError:
                kind = "text"
            else:
                # The raw link contains an unmasked UPI ID, so it is not echoed.
                out.append(DecodedCode(format=c.format, kind=kind, upi=UPIOut(**intent.as_dict())))
                continue
        out.append(DecodedCode(format=c.format, kind=kind, text=c.text))
    for code in out:
        CODES.labels(code.kind).inc()
    return DecodeResponse(codes=out)
