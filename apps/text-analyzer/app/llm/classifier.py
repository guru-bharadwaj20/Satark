"""LLM classification with graceful degradation.

The LLM is an *optional* signal. Any timeout, provider error or invalid output
returns an outcome with ``fallback=True`` so the caller answers from the rule
engine alone. A simple circuit breaker stops calling a provider that keeps
failing, so a provider outage does not add latency to every scan.
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass
from enum import StrEnum

from app.llm.prompts import PROMPT_VERSION, build_classification_messages
from app.llm.provider import (
    CompletionRequest,
    CompletionResponse,
    LLMError,
    LLMProvider,
    LLMTimeoutError,
)
from app.llm.schema import ClassificationResult, SchemaError, parse_classification

log = logging.getLogger("satark.text_analyzer.llm")


class FallbackReason(StrEnum):
    NONE = "none"
    TIMEOUT = "timeout"
    PROVIDER_ERROR = "provider_error"
    INVALID_OUTPUT = "invalid_output"
    CIRCUIT_OPEN = "circuit_open"
    DISABLED = "disabled"


@dataclass(frozen=True, slots=True)
class LLMOutcome:
    result: ClassificationResult | None
    fallback_reason: FallbackReason = FallbackReason.NONE
    provider: str = ""
    model: str = ""
    input_tokens: int = 0
    output_tokens: int = 0
    latency_s: float = 0.0
    cached: bool = False
    prompt_version: str = PROMPT_VERSION

    @property
    def fallback(self) -> bool:
        return self.result is None


class CircuitBreaker:
    def __init__(self, threshold: int = 5, cooldown_s: float = 60.0, clock=time.monotonic):
        self.threshold, self.cooldown_s, self._clock = threshold, cooldown_s, clock
        self._failures = 0
        self._opened_at: float | None = None

    def allow(self) -> bool:
        if self._opened_at is None:
            return True
        if self._clock() - self._opened_at >= self.cooldown_s:
            return True  # half-open: let one request probe the provider
        return False

    def success(self) -> None:
        self._failures, self._opened_at = 0, None

    def failure(self) -> None:
        self._failures += 1
        if self._failures >= self.threshold:
            self._opened_at = self._clock()


class LLMClassifier:
    def __init__(
        self,
        provider: LLMProvider | None,
        *,
        timeout_s: float = 6.0,
        max_tokens: int = 400,
        retry_invalid: int = 1,
        breaker: CircuitBreaker | None = None,
    ) -> None:
        self.provider = provider
        self.timeout_s = timeout_s
        self.max_tokens = max_tokens
        self.retry_invalid = retry_invalid
        self.breaker = breaker or CircuitBreaker()

    def _fallback(self, reason: FallbackReason, **kw) -> LLMOutcome:
        return LLMOutcome(
            result=None,
            fallback_reason=reason,
            provider=getattr(self.provider, "name", ""),
            model=getattr(self.provider, "model", ""),
            **kw,
        )

    async def _call(self, request: CompletionRequest) -> CompletionResponse:
        assert self.provider is not None
        # Belt and braces: enforce the deadline even if a provider ignores it.
        try:
            return await asyncio.wait_for(self.provider.complete(request), self.timeout_s + 0.5)
        except TimeoutError as exc:
            raise LLMTimeoutError("deadline exceeded") from exc

    async def classify(
        self, redacted_text: str, rule_hits: list[str] | None = None, language: str = "en"
    ) -> LLMOutcome:
        if self.provider is None:
            return self._fallback(FallbackReason.DISABLED)
        if not self.breaker.allow():
            return self._fallback(FallbackReason.CIRCUIT_OPEN)

        request = CompletionRequest(
            messages=build_classification_messages(redacted_text, rule_hits, language),
            max_tokens=self.max_tokens,
            temperature=0.0,
            json_mode=True,
            timeout_s=self.timeout_s,
        )
        in_tok = out_tok = 0
        started = time.perf_counter()
        for attempt in range(self.retry_invalid + 1):
            try:
                resp = await self._call(request)
            except LLMTimeoutError:
                self.breaker.failure()
                log.warning("llm timeout", extra={"provider": self.provider.name})
                return self._fallback(
                    FallbackReason.TIMEOUT, input_tokens=in_tok, output_tokens=out_tok
                )
            except LLMError as exc:
                self.breaker.failure()
                log.warning("llm error: %s", exc, extra={"provider": self.provider.name})
                return self._fallback(
                    FallbackReason.PROVIDER_ERROR, input_tokens=in_tok, output_tokens=out_tok
                )
            in_tok += resp.input_tokens
            out_tok += resp.output_tokens
            try:
                result = parse_classification(resp.text)
            except SchemaError as exc:
                log.info("llm invalid output (attempt %d): %s", attempt + 1, exc)
                continue
            self.breaker.success()
            return LLMOutcome(
                result=result,
                provider=resp.provider,
                model=resp.model,
                input_tokens=in_tok,
                output_tokens=out_tok,
                latency_s=time.perf_counter() - started,
            )
        # A provider that answers but never validates is not "down"; don't trip the breaker.
        return self._fallback(
            FallbackReason.INVALID_OUTPUT, input_tokens=in_tok, output_tokens=out_tok
        )
