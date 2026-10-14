"""Ollama provider for locally hosted models (``/api/chat``)."""

from __future__ import annotations

import os
import time

import httpx

from app.llm.provider import (
    CompletionRequest,
    CompletionResponse,
    LLMError,
    LLMProvider,
    LLMTimeoutError,
    register_provider,
)

DEFAULT_URL = "http://ollama.satark.svc.cluster.local:11434"
DEFAULT_MODEL = "qwen2.5:3b-instruct"


class OllamaProvider(LLMProvider):
    name = "ollama"

    def __init__(
        self,
        model: str = DEFAULT_MODEL,
        base_url: str = DEFAULT_URL,
        *,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        super().__init__(model)
        self.base_url = base_url.rstrip("/")
        self._client = client or httpx.AsyncClient(base_url=self.base_url)

    async def complete(self, request: CompletionRequest) -> CompletionResponse:
        payload: dict = {
            "model": self.model,
            "messages": [{"role": m.role, "content": m.content} for m in request.messages],
            "stream": False,
            "options": {"temperature": request.temperature, "num_predict": request.max_tokens},
        }
        if request.json_mode:
            payload["format"] = "json"
        start = time.perf_counter()
        try:
            resp = await self._client.post(
                f"{self.base_url}/api/chat", json=payload, timeout=request.timeout_s
            )
        except httpx.TimeoutException as exc:
            raise LLMTimeoutError(f"ollama timed out after {request.timeout_s}s") from exc
        except httpx.HTTPError as exc:
            raise LLMError(f"ollama request failed: {exc}") from exc
        if resp.status_code != 200:
            raise LLMError(f"ollama returned HTTP {resp.status_code}: {resp.text[:200]}")
        try:
            data = resp.json()
            text = data["message"]["content"]
        except (ValueError, KeyError, TypeError) as exc:
            raise LLMError("ollama returned an unexpected payload") from exc
        return CompletionResponse(
            text=text,
            model=data.get("model", self.model),
            provider=self.name,
            input_tokens=int(data.get("prompt_eval_count", 0)),
            output_tokens=int(data.get("eval_count", 0)),
            latency_s=time.perf_counter() - start,
            raw=data,
        )

    async def aclose(self) -> None:
        await self._client.aclose()


def _factory() -> LLMProvider:
    return OllamaProvider(
        model=os.environ.get("SATARK_OLLAMA_MODEL", DEFAULT_MODEL),
        base_url=os.environ.get("SATARK_OLLAMA_URL", DEFAULT_URL),
    )


register_provider("ollama", _factory)
