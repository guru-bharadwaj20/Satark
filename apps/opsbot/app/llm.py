"""Minimal LLM client for opsbot.

Every prompt passes through app.redact before it is sent, and token usage is
reported so the daily budget can be enforced.
"""

from __future__ import annotations

import logging
import time
from dataclasses import dataclass

import httpx
from prometheus_client import Counter

from app.redact import redact

log = logging.getLogger("satark.opsbot.llm")

LLM_TOKENS = Counter(
    "satark_llm_tokens_total", "LLM tokens consumed.", ["provider", "model", "direction"]
)
LLM_REQUESTS = Counter(
    "satark_llm_requests_total", "LLM requests by outcome.", ["provider", "model", "outcome"]
)
REDACTIONS = Counter("satark_opsbot_redactions_total", "Values redacted from prompts.", ["kind"])


class LLMError(Exception):
    pass


@dataclass(frozen=True)
class LLMReply:
    text: str
    input_tokens: int
    output_tokens: int

    @property
    def total_tokens(self) -> int:
        return self.input_tokens + self.output_tokens


class LLMClient:
    def __init__(
        self,
        provider: str,
        base_url: str,
        model: str,
        api_key: str = "",
        timeout_s: float = 45.0,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        if provider not in {"ollama", "anthropic", "openai-compatible"}:
            raise ValueError(f"unsupported provider {provider}")
        self.provider, self.model, self.api_key = provider, model, api_key
        self.base_url = base_url.rstrip("/")
        self.timeout_s = timeout_s
        self._client = client or httpx.AsyncClient()

    async def chat(
        self, system: str, user: str, max_tokens: int = 800, json_mode: bool = False
    ) -> LLMReply:
        system, c1 = redact(system)
        user, c2 = redact(user)
        for kind, n in (c1 + c2).items():
            REDACTIONS.labels(kind).inc(n)
        start = time.perf_counter()
        try:
            reply = await self._send(system, user, max_tokens, json_mode)
        except (httpx.HTTPError, KeyError, ValueError, TypeError) as exc:
            LLM_REQUESTS.labels(self.provider, self.model, "error").inc()
            raise LLMError(f"{self.provider}: {type(exc).__name__}") from exc
        LLM_REQUESTS.labels(self.provider, self.model, "ok").inc()
        LLM_TOKENS.labels(self.provider, self.model, "input").inc(reply.input_tokens)
        LLM_TOKENS.labels(self.provider, self.model, "output").inc(reply.output_tokens)
        log.info(
            "llm call ok in %.1fs (%d tokens)", time.perf_counter() - start, reply.total_tokens
        )
        return reply

    async def _send(self, system: str, user: str, max_tokens: int, json_mode: bool) -> LLMReply:
        if self.provider == "ollama":
            body = {
                "model": self.model,
                "stream": False,
                "messages": [
                    {"role": "system", "content": system},
                    {"role": "user", "content": user},
                ],
                "options": {"temperature": 0.1, "num_predict": max_tokens},
            }
            if json_mode:
                body["format"] = "json"
            r = await self._client.post(
                f"{self.base_url}/api/chat", json=body, timeout=self.timeout_s
            )
            r.raise_for_status()
            d = r.json()
            return LLMReply(
                d["message"]["content"],
                int(d.get("prompt_eval_count", 0)),
                int(d.get("eval_count", 0)),
            )
        if self.provider == "anthropic":
            r = await self._client.post(
                f"{self.base_url}/v1/messages",
                headers={"x-api-key": self.api_key, "anthropic-version": "2023-06-01"},
                json={
                    "model": self.model,
                    "max_tokens": max_tokens,
                    "temperature": 0.1,
                    "system": system,
                    "messages": [{"role": "user", "content": user}],
                },
                timeout=self.timeout_s,
            )
            r.raise_for_status()
            d = r.json()
            text = "".join(b.get("text", "") for b in d["content"] if b.get("type") == "text")
            u = d.get("usage", {})
            return LLMReply(text, int(u.get("input_tokens", 0)), int(u.get("output_tokens", 0)))
        body = {
            "model": self.model,
            "max_tokens": max_tokens,
            "temperature": 0.1,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
        }
        if json_mode:
            body["response_format"] = {"type": "json_object"}
        r = await self._client.post(
            f"{self.base_url}/v1/chat/completions",
            json=body,
            headers={"Authorization": f"Bearer {self.api_key}"},
            timeout=self.timeout_s,
        )
        r.raise_for_status()
        d = r.json()
        u = d.get("usage", {})
        return LLMReply(
            d["choices"][0]["message"]["content"] or "",
            int(u.get("prompt_tokens", 0)),
            int(u.get("completion_tokens", 0)),
        )
