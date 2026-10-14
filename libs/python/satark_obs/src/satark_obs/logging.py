"""Structured JSON logging matching libs/go/observability.

Every line is one JSON object: time, level, msg, service, version, env,
trace_id (inside a request) plus any ``extra=`` fields. Sensitive keys are
redacted and phone numbers / UPI handles are masked in all string values.
"""

from __future__ import annotations

import json
import logging
import os
import re
import secrets
import sys
import time
from contextvars import ContextVar
from datetime import UTC, datetime

from starlette.types import ASGIApp, Message, Receive, Scope, Send

TRACE_HEADER = "x-request-id"
REDACTED = "[REDACTED]"
SENSITIVE_KEYS = frozenset(
    {
        "text", "message_text", "body", "phone", "phone_number", "upi", "upi_id", "email",
        "token", "password", "authorization", "api_key", "secret", "chat_id",
    }
)
_PHONE_RE = re.compile(r"(?:\+?91[\s-]?)?[6-9]\d{4}[\s-]?\d{5}")
_UPI_RE = re.compile(r"[\w.\-]{2,}@[a-zA-Z]{2,}")

trace_id_var: ContextVar[str] = ContextVar("trace_id", default="")

# Attributes every LogRecord has; anything else came from ``extra=``.
_RESERVED = frozenset(vars(logging.LogRecord("", 0, "", 0, "", None, None))) | {"message", "asctime", "taskName"}


def mask_pii(value: str) -> str:
    return _UPI_RE.sub("[UPI]", _PHONE_RE.sub("[PHONE]", value))


def _clean(key: str, value):
    if key.lower() in SENSITIVE_KEYS:
        return REDACTED
    if isinstance(value, str):
        return mask_pii(value)
    if isinstance(value, (int, float, bool)) or value is None:
        return value
    return mask_pii(str(value))


class JSONFormatter(logging.Formatter):
    def __init__(self, service: str, version: str = "", env: str = "") -> None:
        super().__init__()
        self.static = {"service": service, "version": version, "env": env}

    def format(self, record: logging.LogRecord) -> str:
        out = {
            "time": datetime.fromtimestamp(record.created, UTC).isoformat(timespec="milliseconds"),
            "level": record.levelname if record.levelname != "WARNING" else "WARN",
            "msg": mask_pii(record.getMessage()),
            **self.static,
            "logger": record.name,
        }
        if tid := trace_id_var.get():
            out["trace_id"] = tid
        for key, value in record.__dict__.items():
            if key not in _RESERVED and not key.startswith("_"):
                out[key] = _clean(key, value)
        if record.exc_info:
            out["error"] = mask_pii(self.formatException(record.exc_info))
        return json.dumps(out, ensure_ascii=False)


def configure_logging(service: str, level: str | None = None, stream=None) -> logging.Logger:
    """Install the JSON formatter on the root logger (and uvicorn's loggers)."""
    handler = logging.StreamHandler(stream or sys.stdout)
    handler.setFormatter(
        JSONFormatter(service, os.getenv("SATARK_VERSION", ""), os.getenv("SATARK_ENV", ""))
    )
    root = logging.getLogger()
    root.handlers[:] = [handler]
    root.setLevel((level or os.getenv("LOG_LEVEL", "INFO")).upper())
    for name in ("uvicorn", "uvicorn.error"):
        lg = logging.getLogger(name)
        lg.handlers.clear()
        lg.propagate = True
    # Access logs come from AccessLogMiddleware instead (no query strings).
    logging.getLogger("uvicorn.access").disabled = True
    return logging.getLogger(service)


class AccessLogMiddleware:
    """Propagate X-Request-ID into ``trace_id_var`` and log one line per request."""

    QUIET = ("/healthz", "/readyz", "/metrics")

    def __init__(self, app: ASGIApp, logger: logging.Logger | None = None) -> None:
        self.app = app
        self.logger = logger or logging.getLogger("access")

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        incoming = dict(scope.get("headers") or []).get(TRACE_HEADER.encode(), b"").decode("latin-1")
        tid = incoming if 0 < len(incoming) <= 64 else secrets.token_hex(16)
        token = trace_id_var.set(tid)
        status = 500
        start = time.perf_counter()

        async def send_wrapper(message: Message) -> None:
            nonlocal status
            if message["type"] == "http.response.start":
                status = message["status"]
                message.setdefault("headers", [])
                message["headers"] = [*message["headers"], (TRACE_HEADER.encode(), tid.encode())]
            await send(message)

        try:
            await self.app(scope, receive, send_wrapper)
        finally:
            if scope["path"] not in self.QUIET:
                self.logger.log(
                    logging.ERROR if status >= 500 else logging.INFO,
                    "request",
                    extra={
                        "method": scope["method"],
                        "path": scope["path"],
                        "status": status,
                        "duration_ms": round((time.perf_counter() - start) * 1000, 3),
                    },
                )
            trace_id_var.reset(token)
