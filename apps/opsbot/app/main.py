"""opsbot HTTP service: Alertmanager webhook receiver plus health and metrics."""

from __future__ import annotations

import asyncio
import hmac
import logging
from collections.abc import Awaitable, Callable
from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI, Header, HTTPException, Response, status
from prometheus_client import CONTENT_TYPE_LATEST, Counter, generate_latest

from app.alerts import Alert, AlertmanagerPayload
from app.config import Settings, get_settings

log = logging.getLogger("satark.opsbot")

ALERTS_RECEIVED = Counter(
    "satark_opsbot_alerts_received_total", "Alerts received from Alertmanager.", ["status"]
)
ALERTS_DROPPED = Counter("satark_opsbot_alerts_dropped_total", "Alerts not diagnosed.", ["reason"])

AlertHandler = Callable[[Alert], Awaitable[None]]


async def _log_only(alert: Alert) -> None:
    log.info("alert received: %s (%s)", alert.name, alert.namespace)


class AlertQueue:
    """Bounded queue with one worker so diagnoses never block the webhook."""

    def __init__(self, handler: AlertHandler, maxsize: int = 100) -> None:
        self.handler = handler
        self.queue: asyncio.Queue[Alert] = asyncio.Queue(maxsize=maxsize)
        self._task: asyncio.Task | None = None

    def offer(self, alert: Alert) -> bool:
        try:
            self.queue.put_nowait(alert)
            return True
        except asyncio.QueueFull:
            return False

    async def _run(self) -> None:
        while True:
            alert = await self.queue.get()
            try:
                await self.handler(alert)
            except Exception:  # keep the worker alive
                log.exception("diagnosis failed for %s", alert.name)
            finally:
                self.queue.task_done()

    def start(self) -> None:
        if self._task is None:
            self._task = asyncio.create_task(self._run(), name="opsbot-worker")

    async def stop(self) -> None:
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None


def create_app(
    handler: AlertHandler | None = None,
    settings: Settings | None = None,
    router=None,
) -> FastAPI:
    settings = settings or get_settings()
    queue = AlertQueue(handler or _log_only, settings.queue_size)

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        queue.start()
        yield
        await queue.stop()

    app = FastAPI(title="opsbot", version="0.1.0", lifespan=lifespan)
    app.state.queue = queue
    app.state.settings = settings
    app.state.router = router
    app.state.tasks = set()

    def require_token(authorization: str = Header(default="")) -> None:
        expected = settings.webhook_token.get_secret_value()
        if not expected:
            raise HTTPException(status.HTTP_503_SERVICE_UNAVAILABLE, "webhook token not configured")
        supplied = authorization.removeprefix("Bearer ").strip()
        if not hmac.compare_digest(supplied.encode(), expected.encode()):
            raise HTTPException(status.HTTP_401_UNAUTHORIZED, "invalid token")

    @app.get("/healthz")
    async def healthz() -> dict:
        return {"status": "ok"}

    @app.get("/readyz")
    async def readyz() -> dict:
        if queue.queue.full():
            raise HTTPException(status.HTTP_503_SERVICE_UNAVAILABLE, "queue full")
        return {"status": "ready"}

    @app.get("/metrics")
    async def metrics() -> Response:
        return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)

    @app.post(
        "/v1/alertmanager",
        status_code=status.HTTP_202_ACCEPTED,
        dependencies=[Depends(require_token)],
    )
    async def alertmanager(payload: AlertmanagerPayload) -> dict:
        accepted = 0
        for alert in payload.alerts:
            ALERTS_RECEIVED.labels(alert.status).inc()
            if alert.status != "firing":
                ALERTS_DROPPED.labels("resolved").inc()
                continue
            if (
                settings.namespaces
                and alert.namespace
                and alert.namespace not in settings.namespaces
            ):
                ALERTS_DROPPED.labels("namespace").inc()
                continue
            if queue.offer(alert):
                accepted += 1
            else:
                ALERTS_DROPPED.labels("queue_full").inc()
        return {"accepted": accepted, "received": len(payload.alerts)}

    @app.post("/v1/telegram")
    async def telegram_update(
        update: dict,
        x_telegram_bot_api_secret_token: str = Header(default=""),
    ) -> dict:
        expected = settings.telegram_webhook_secret.get_secret_value()
        if not expected or not hmac.compare_digest(
            x_telegram_bot_api_secret_token.encode(), expected.encode()
        ):
            raise HTTPException(status.HTTP_401_UNAUTHORIZED, "invalid secret")
        if router is None:
            return {"ok": True, "handled": False}
        # Answer Telegram immediately; LLM work happens in the background.
        task = asyncio.create_task(router.handle_update(update))
        app.state.tasks.add(task)
        task.add_done_callback(app.state.tasks.discard)
        return {"ok": True, "handled": True}

    return app


def build() -> FastAPI:  # pragma: no cover - wiring for uvicorn
    from app.pipeline import build_handler, build_router

    s = get_settings()
    handler = build_handler(s)
    return create_app(handler, s, build_router(s, handler))
