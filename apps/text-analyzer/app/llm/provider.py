"""Provider-neutral LLM interface.

Concrete providers (Ollama, Bedrock, hosted APIs) implement :class:`LLMProvider`
and are looked up by name through a small registry, so the provider can be
switched with the ``SATARK_LLM_PROVIDER`` environment variable without code
changes.

Every request that goes through this interface must contain text that has
already been PII-redacted.
"""

from __future__ import annotations

import abc
import importlib
import os
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Literal

Role = Literal["system", "user", "assistant"]


class LLMError(Exception):
    """Any failure talking to an LLM provider (network, HTTP, bad payload)."""


class LLMTimeoutError(LLMError):
    """The provider did not answer within the configured deadline."""


@dataclass(frozen=True, slots=True)
class Message:
    role: Role
    content: str


@dataclass(frozen=True, slots=True)
class CompletionRequest:
    messages: tuple[Message, ...]
    max_tokens: int = 512
    temperature: float = 0.0
    json_mode: bool = True
    timeout_s: float = 8.0

    @property
    def system(self) -> str:
        return "\n\n".join(m.content for m in self.messages if m.role == "system")

    @property
    def conversation(self) -> list[Message]:
        return [m for m in self.messages if m.role != "system"]


@dataclass(frozen=True, slots=True)
class CompletionResponse:
    text: str
    model: str
    provider: str
    input_tokens: int = 0
    output_tokens: int = 0
    latency_s: float = 0.0
    raw: dict = field(default_factory=dict, compare=False, repr=False)


class LLMProvider(abc.ABC):
    """Base class for all providers."""

    name: str = "base"

    def __init__(self, model: str) -> None:
        self.model = model

    @abc.abstractmethod
    async def complete(self, request: CompletionRequest) -> CompletionResponse:
        """Run one chat completion. Must raise LLMError/LLMTimeoutError on failure."""

    async def aclose(self) -> None:  # pragma: no cover - default no-op
        return None


ProviderFactory = Callable[[], LLMProvider]
_REGISTRY: dict[str, ProviderFactory] = {}
_BUILTIN = ("app.llm.ollama", "app.llm.bedrock", "app.llm.hosted")


def register_provider(name: str, factory: ProviderFactory) -> None:
    _REGISTRY[name] = factory


def available_providers() -> list[str]:
    _load_builtin()
    return sorted(_REGISTRY)


def get_provider(name: str | None = None) -> LLMProvider:
    """Build the provider named ``name`` (default: ``$SATARK_LLM_PROVIDER``)."""
    name = name or os.environ.get("SATARK_LLM_PROVIDER", "ollama")
    _load_builtin()
    try:
        factory = _REGISTRY[name]
    except KeyError as exc:
        known = ", ".join(sorted(_REGISTRY)) or "none"
        raise LLMError(f"unknown LLM provider {name!r} (known: {known})") from exc
    return factory()


DISABLED_NAMES = {"none", "disabled", "off"}


def get_provider_or_none(name: str | None = None) -> LLMProvider | None:
    """Like get_provider, but SATARK_LLM_PROVIDER=none disables the LLM (rules-only)."""
    name = name or os.environ.get("SATARK_LLM_PROVIDER", "ollama")
    if name.lower() in DISABLED_NAMES:
        return None
    return get_provider(name)


def _load_builtin() -> None:
    """Import built-in provider modules so they self-register.

    Modules that do not exist yet are skipped; optional third-party
    dependencies are imported lazily inside each provider.
    """
    for mod in _BUILTIN:
        try:
            importlib.import_module(mod)
        except ModuleNotFoundError as exc:
            if exc.name == mod:
                continue
            raise
