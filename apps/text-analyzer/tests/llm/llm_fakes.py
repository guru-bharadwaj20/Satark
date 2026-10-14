"""Test doubles for LLM providers."""

from __future__ import annotations

import asyncio

from app.llm.provider import CompletionRequest, CompletionResponse, LLMError, LLMProvider


class FakeProvider(LLMProvider):
    name = "fake"

    def __init__(self, replies=None, *, delay: float = 0.0, error: Exception | None = None):
        super().__init__("fake-model")
        self.replies = list(replies or [])
        self.delay = delay
        self.error = error
        self.calls: list[CompletionRequest] = []

    async def complete(self, request: CompletionRequest) -> CompletionResponse:
        self.calls.append(request)
        if self.delay:
            await asyncio.sleep(self.delay)
        if self.error:
            raise self.error
        if not self.replies:
            raise LLMError("no scripted reply")
        text = self.replies.pop(0) if len(self.replies) > 1 else self.replies[0]
        return CompletionResponse(
            text=text, model=self.model, provider=self.name, input_tokens=100, output_tokens=40
        )
