"""Hosted API providers.

* ``anthropic`` - the Anthropic Messages API.
* ``openai-compatible`` - any server exposing ``/v1/chat/completions``
  (vLLM, LiteLLM, many hosted gateways).

API keys are read from environment variables that are populated from a
Kubernetes Secret; they are never logged.
"""

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


class _HTTPProvider(LLMProvider):
    def __init__(self, model: str, base_url: str, api_key: str, *, client=None) -> None:
        super().__init__(model)
        if not api_key:
            raise LLMError(f"{self.name}: API key is not configured")
        self.base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._client = client or httpx.AsyncClient()

    async def _post(self, path: str, payload: dict, headers: dict, timeout: float) -> dict:
        try:
            resp = await self._client.post(
                f"{self.base_url}{path}", json=payload, headers=headers, timeout=timeout
            )
        except httpx.TimeoutException as exc:
            raise LLMTimeoutError(f"{self.name} timed out after {timeout}s") from exc
        except httpx.HTTPError as exc:
            raise LLMError(f"{self.name} request failed: {type(exc).__name__}") from exc
        if resp.status_code != 200:
            raise LLMError(f"{self.name} returned HTTP {resp.status_code}")
        try:
            return resp.json()
        except ValueError as exc:
            raise LLMError(f"{self.name} returned invalid JSON") from exc

    async def aclose(self) -> None:
        await self._client.aclose()


class AnthropicProvider(_HTTPProvider):
    name = "anthropic"

    async def complete(self, request: CompletionRequest) -> CompletionResponse:
        payload: dict = {
            "model": self.model,
            "max_tokens": request.max_tokens,
            "temperature": request.temperature,
            "messages": [{"role": m.role, "content": m.content} for m in request.conversation],
        }
        if request.system:
            payload["system"] = request.system
        headers = {"x-api-key": self._api_key, "anthropic-version": "2023-06-01"}
        start = time.perf_counter()
        data = await self._post("/v1/messages", payload, headers, request.timeout_s)
        try:
            text = "".join(b["text"] for b in data["content"] if b.get("type") == "text")
            usage = data.get("usage", {})
        except (KeyError, TypeError) as exc:
            raise LLMError("anthropic returned an unexpected payload") from exc
        return CompletionResponse(
            text=text,
            model=data.get("model", self.model),
            provider=self.name,
            input_tokens=int(usage.get("input_tokens", 0)),
            output_tokens=int(usage.get("output_tokens", 0)),
            latency_s=time.perf_counter() - start,
            raw=data,
        )


class OpenAICompatibleProvider(_HTTPProvider):
    name = "openai-compatible"

    async def complete(self, request: CompletionRequest) -> CompletionResponse:
        payload: dict = {
            "model": self.model,
            "max_tokens": request.max_tokens,
            "temperature": request.temperature,
            "messages": [{"role": m.role, "content": m.content} for m in request.messages],
        }
        if request.json_mode:
            payload["response_format"] = {"type": "json_object"}
        headers = {"Authorization": f"Bearer {self._api_key}"}
        start = time.perf_counter()
        data = await self._post("/v1/chat/completions", payload, headers, request.timeout_s)
        try:
            text = data["choices"][0]["message"]["content"] or ""
            usage = data.get("usage", {})
        except (KeyError, IndexError, TypeError) as exc:
            raise LLMError("openai-compatible returned an unexpected payload") from exc
        return CompletionResponse(
            text=text,
            model=data.get("model", self.model),
            provider=self.name,
            input_tokens=int(usage.get("prompt_tokens", 0)),
            output_tokens=int(usage.get("completion_tokens", 0)),
            latency_s=time.perf_counter() - start,
            raw=data,
        )


register_provider(
    "anthropic",
    lambda: AnthropicProvider(
        model=os.environ.get("SATARK_ANTHROPIC_MODEL", "claude-haiku-4-5"),
        base_url=os.environ.get("SATARK_ANTHROPIC_URL", "https://api.anthropic.com"),
        api_key=os.environ.get("ANTHROPIC_API_KEY", ""),
    ),
)
register_provider(
    "openai-compatible",
    lambda: OpenAICompatibleProvider(
        model=os.environ.get("SATARK_OPENAI_COMPAT_MODEL", ""),
        base_url=os.environ.get("SATARK_OPENAI_COMPAT_URL", "http://localhost:8000"),
        api_key=os.environ.get("SATARK_OPENAI_COMPAT_API_KEY", ""),
    ),
)
