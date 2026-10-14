"""Turn an alert into a diagnosis: context + runbooks -> LLM -> message."""

from __future__ import annotations

import logging
from dataclasses import dataclass, field

from app.alerts import Alert
from app.context import ContextGatherer, IncidentContext
from app.llm import LLMClient, LLMError
from app.rag.retrieve import Retriever, format_sources
from app.rag.store import Hit
from app.telegram import escape

log = logging.getLogger("satark.opsbot.diagnose")

SYSTEM = """You are opsbot, the on-call assistant for the Satark Kubernetes platform.
Diagnose the alert using ONLY the cluster context and runbook sections provided.

Rules:
- You are read-only. Suggest commands; never claim you ran anything.
- Prefer read-only commands (kubectl get/describe/logs, PromQL, LogQL). Mark any command
  that changes state with "(changes state)" and say it needs a human.
- Cite runbook sections as [1], [2]. If no runbook fits, say so.
- If the context is insufficient, say what is missing instead of guessing.
- Be concise: on-call engineers read this on a phone.

Format:
Likely cause: <one sentence>
Confidence: <low|medium|high>
Evidence:
- <fact from the context>
Next steps:
1. <step, with command>
Runbooks: <citations>"""


@dataclass
class Diagnosis:
    alert: Alert
    text: str
    sources: list[Hit] = field(default_factory=list)
    tokens: int = 0
    llm_ok: bool = True


class Diagnoser:
    def __init__(self, gatherer: ContextGatherer, retriever: Retriever | None, llm: LLMClient):
        self.gatherer, self.retriever, self.llm = gatherer, retriever, llm

    async def diagnose(self, alert: Alert) -> Diagnosis:
        ctx = await self.gatherer.gather(alert)
        ctx_text = ctx.render()
        hits: list[Hit] = []
        if self.retriever is not None:
            try:
                hits = await self.retriever.for_alert(alert, ctx_text)
            except Exception as exc:
                log.warning("runbook retrieval failed: %s", type(exc).__name__)
        user = f"CLUSTER CONTEXT\n{ctx_text}\n\nRUNBOOK SECTIONS\n{format_sources(hits)}"
        try:
            reply = await self.llm.chat(SYSTEM, user, max_tokens=700)
        except LLMError as exc:
            log.warning("llm unavailable, posting context only: %s", exc)
            return Diagnosis(alert, fallback_text(ctx, hits), hits, 0, llm_ok=False)
        return Diagnosis(alert, reply.text.strip(), hits, reply.total_tokens)


def fallback_text(ctx: IncidentContext, hits: list[Hit]) -> str:
    lines = ["LLM unavailable - raw context only.", ""]
    for name, body in list(ctx.sections.items())[:3]:
        lines += [f"{name}:", "\n".join(body.splitlines()[:8]), ""]
    if hits:
        lines.append("Runbooks: " + ", ".join(f"{h.title} - {h.heading}" for h in hits))
    return "\n".join(lines).strip()


def format_for_telegram(d: Diagnosis) -> str:
    a = d.alert
    icon = {"critical": "\U0001f534", "warning": "\U0001f7e0"}.get(a.severity, "\U0001f535")
    head = (
        f"{icon} <b>{escape(a.name)}</b> [{escape(a.severity)}]"
        f" ns=<code>{escape(a.namespace or '-')}</code>"
    )
    target = a.labels.get("pod") or a.labels.get("deployment") or a.labels.get("node")
    if target:
        head += f" <code>{escape(target)}</code>"
    footer = "\n\n<i>AI-generated from read-only context; verify before acting.</i>"
    if d.tokens:
        footer += f" <i>({d.tokens} tokens)</i>"
    return f"{head}\n\n{escape(d.text)}{footer}"
