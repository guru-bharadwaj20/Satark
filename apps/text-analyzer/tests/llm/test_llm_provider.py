import asyncio

import pytest
from llm_fakes import FakeProvider

from app.llm.provider import (
    CompletionRequest,
    LLMError,
    Message,
    get_provider,
    register_provider,
)


def test_request_splits_system_and_conversation():
    req = CompletionRequest(
        messages=(Message("system", "a"), Message("system", "b"), Message("user", "hi"))
    )
    assert req.system == "a\n\nb"
    assert [m.content for m in req.conversation] == ["hi"]


def test_registry_lookup_and_unknown():
    register_provider("fake", lambda: FakeProvider(['{"x":1}']))
    p = get_provider("fake")
    resp = asyncio.run(p.complete(CompletionRequest(messages=(Message("user", "x"),))))
    assert resp.text == '{"x":1}'
    with pytest.raises(LLMError):
        get_provider("does-not-exist")


def test_env_selects_provider(monkeypatch):
    register_provider("fake-env", lambda: FakeProvider(["ok"]))
    monkeypatch.setenv("SATARK_LLM_PROVIDER", "fake-env")
    assert get_provider().name == "fake"
