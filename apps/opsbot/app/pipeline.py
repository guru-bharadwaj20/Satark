"""Wire the alert pipeline: diagnose an alert and post it to the ops channel."""

from __future__ import annotations

import logging

from prometheus_client import Counter

from app.alerts import Alert
from app.config import Settings
from app.diagnose import Diagnoser, format_for_telegram

log = logging.getLogger("satark.opsbot.pipeline")

DIAGNOSES = Counter("satark_opsbot_diagnoses_total", "Diagnoses posted.", ["result"])


class AlertPipeline:
    def __init__(self, diagnoser: Diagnoser, telegram, chat_id: str) -> None:
        self.diagnoser, self.telegram, self.chat_id = diagnoser, telegram, chat_id

    async def __call__(self, alert: Alert) -> None:
        d = await self.diagnoser.diagnose(alert)
        await self.telegram.send(self.chat_id, format_for_telegram(d))
        DIAGNOSES.labels("llm" if d.llm_ok else "fallback").inc()


def build_handler(s: Settings):  # pragma: no cover - production wiring
    from app.context import ContextGatherer, KubeClient, LokiClient
    from app.llm import LLMClient
    from app.telegram import TelegramClient

    kube = KubeClient.in_cluster(s.kube_api, s.kube_token_path, s.kube_ca_path)
    gatherer = ContextGatherer(kube, LokiClient(s.loki_url))
    llm = LLMClient(
        s.llm_provider, s.llm_url, s.llm_model, s.llm_api_key.get_secret_value(), s.llm_timeout_s
    )
    retriever = None
    if s.database_url.get_secret_value():
        retriever = _lazy_retriever(s)
    tg = TelegramClient(s.telegram_bot_token.get_secret_value())
    return AlertPipeline(Diagnoser(gatherer, retriever, llm), tg, s.telegram_ops_chat_id)


class _lazy_retriever:  # pragma: no cover - connects on first use
    def __init__(self, s: Settings) -> None:
        self.s, self._r = s, None

    async def for_alert(self, alert, context_text=""):
        if self._r is None:
            from app.rag.embed import OllamaEmbedder
            from app.rag.retrieve import Retriever
            from app.rag.store import PgVectorStore

            store = await PgVectorStore.connect(
                self.s.database_url.get_secret_value(), self.s.embed_dim
            )
            self._r = Retriever(
                OllamaEmbedder(self.s.llm_url, self.s.embed_model, self.s.embed_dim), store
            )
        return await self._r.for_alert(alert, context_text)
