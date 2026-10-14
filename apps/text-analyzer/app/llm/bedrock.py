"""Amazon Bedrock provider using the Converse API.

``boto3`` is an optional dependency and is imported lazily. Credentials come
from the standard AWS chain; in-cluster that is the role created by the
``bedrock-access`` Terraform module.
"""

from __future__ import annotations

import asyncio
import os
import time
from typing import Any

from app.llm.provider import (
    CompletionRequest,
    CompletionResponse,
    LLMError,
    LLMProvider,
    LLMTimeoutError,
    register_provider,
)

DEFAULT_MODEL = "anthropic.claude-3-haiku-20240307-v1:0"


class BedrockProvider(LLMProvider):
    name = "bedrock"

    def __init__(
        self, model: str = DEFAULT_MODEL, region: str = "ap-south-1", *, client: Any = None
    ):
        super().__init__(model)
        self.region = region
        self._client = client

    def _get_client(self) -> Any:
        if self._client is None:
            try:
                import boto3
                from botocore.config import Config
            except ModuleNotFoundError as exc:  # pragma: no cover - env specific
                raise LLMError("boto3 is required for the bedrock provider") from exc
            self._client = boto3.client(
                "bedrock-runtime",
                region_name=self.region,
                config=Config(retries={"max_attempts": 2, "mode": "standard"}, read_timeout=10),
            )
        return self._client

    def _converse(self, request: CompletionRequest) -> dict:
        kwargs: dict[str, Any] = {
            "modelId": self.model,
            "messages": [
                {"role": m.role, "content": [{"text": m.content}]} for m in request.conversation
            ],
            "inferenceConfig": {
                "maxTokens": request.max_tokens,
                "temperature": request.temperature,
            },
        }
        if request.system:
            kwargs["system"] = [{"text": request.system}]
        return self._get_client().converse(**kwargs)

    async def complete(self, request: CompletionRequest) -> CompletionResponse:
        start = time.perf_counter()
        try:
            data = await asyncio.wait_for(
                asyncio.to_thread(self._converse, request), timeout=request.timeout_s
            )
        except TimeoutError as exc:
            raise LLMTimeoutError(f"bedrock timed out after {request.timeout_s}s") from exc
        except LLMError:
            raise
        except Exception as exc:  # botocore raises many unrelated types
            raise LLMError(f"bedrock request failed: {type(exc).__name__}: {exc}") from exc
        try:
            blocks = data["output"]["message"]["content"]
            text = "".join(b.get("text", "") for b in blocks)
            usage = data.get("usage", {})
        except (KeyError, TypeError) as exc:
            raise LLMError("bedrock returned an unexpected payload") from exc
        return CompletionResponse(
            text=text,
            model=self.model,
            provider=self.name,
            input_tokens=int(usage.get("inputTokens", 0)),
            output_tokens=int(usage.get("outputTokens", 0)),
            latency_s=time.perf_counter() - start,
            raw=data,
        )


def _factory() -> LLMProvider:
    return BedrockProvider(
        model=os.environ.get("SATARK_BEDROCK_MODEL", DEFAULT_MODEL),
        region=os.environ.get("AWS_REGION", "ap-south-1"),
    )


register_provider("bedrock", _factory)
