import asyncio
import json

import httpx
import pytest

from app.llm.ollama import OllamaProvider
from app.llm.provider import CompletionRequest, LLMError, LLMTimeoutError, Message

REQ = CompletionRequest(messages=(Message("system", "s"), Message("user", "u")), max_tokens=64)


def _provider(handler):
    client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    return OllamaProvider(model="m", base_url="http://ollama:11434", client=client)


def test_success_maps_tokens_and_payload():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen.update(json.loads(request.content))
        assert request.url.path == "/api/chat"
        return httpx.Response(
            200,
            json={"model": "m", "message": {"role": "assistant", "content": "{}"},
                  "prompt_eval_count": 12, "eval_count": 5},
        )

    resp = asyncio.run(_provider(handler).complete(REQ))
    assert resp.text == "{}"
    assert (resp.input_tokens, resp.output_tokens) == (12, 5)
    assert seen["format"] == "json"
    assert seen["stream"] is False
    assert seen["options"]["num_predict"] == 64
    assert [m["role"] for m in seen["messages"]] == ["system", "user"]


def test_http_error_raises():
    p = _provider(lambda r: httpx.Response(500, text="boom"))
    with pytest.raises(LLMError):
        asyncio.run(p.complete(REQ))


def test_timeout_raises_timeout_error():
    def handler(request):
        raise httpx.ReadTimeout("slow", request=request)

    with pytest.raises(LLMTimeoutError):
        asyncio.run(_provider(handler).complete(REQ))


def test_bad_payload_raises():
    p = _provider(lambda r: httpx.Response(200, json={"nope": True}))
    with pytest.raises(LLMError):
        asyncio.run(p.complete(REQ))
