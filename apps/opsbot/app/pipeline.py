"""Wire the alert pipeline: diagnose an alert and post it to the ops channel."""

from __future__ import annotations

import logging

from prometheus_client import Counter

from app.alerts import Alert
from app.config import Settings
from app.diagnose import Diagnoser, format_for_telegram
from app.limits import Cooldown, DailyTokenBudget

log = logging.getLogger("satark.opsbot.pipeline")

DIAGNOSES = Counter("satark_opsbot_diagnoses_total", "Diagnoses posted.", ["result"])


SKIPPED = Counter("satark_opsbot_alerts_skipped_total", "Alerts not diagnosed.", ["reason"])


class AlertPipeline:
    def __init__(
        self,
        diagnoser: Diagnoser,
        telegram,
        chat_id: str,
        cooldown: Cooldown | None = None,
        budget: DailyTokenBudget | None = None,
    ) -> None:
        self.diagnoser, self.telegram, self.chat_id = diagnoser, telegram, chat_id
        self.cooldown = cooldown or Cooldown(1800)
        self.budget = budget or DailyTokenBudget(200_000)
        self._budget_notice_day = None

    async def __call__(self, alert: Alert) -> None:
        if not self.cooldown.try_acquire(alert.key):
            SKIPPED.labels("cooldown").inc()
            log.info("skipping %s: cooling down", alert.name)
            return
        use_llm = self.budget.allows()
        if not use_llm:
            SKIPPED.labels("budget").inc()
        d = await self.diagnoser.diagnose(
            alert, use_llm=use_llm, fallback_note="Daily LLM token budget reached"
        )
        self.budget.record(d.tokens)
        await self.telegram.send(self.chat_id, format_for_telegram(d))
        DIAGNOSES.labels("llm" if d.llm_ok else "fallback").inc()
        if not use_llm and self._budget_notice_day != self.budget._day:
            self._budget_notice_day = self.budget._day
            await self.telegram.send(
                self.chat_id,
                "Daily LLM token budget reached; diagnoses are context-only until 00:00 UTC "
                "(see RB-012).",
            )


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
    return AlertPipeline(
        Diagnoser(gatherer, retriever, llm),
        tg,
        s.telegram_ops_chat_id,
        Cooldown(s.cooldown_s),
        DailyTokenBudget(s.daily_token_budget),
    )


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
