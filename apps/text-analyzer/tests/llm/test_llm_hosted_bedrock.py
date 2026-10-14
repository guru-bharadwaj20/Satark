import asyncio
import json
import time

import httpx
import pytest

from app.llm.bedrock import BedrockProvider
from app.llm.hosted import AnthropicProvider, OpenAICompatibleProvider
from app.llm.provider import CompletionRequest, LLMError, LLMTimeoutError, Message

REQ = CompletionRequest(messages=(Message("system", "sys"), Message("user", "hello")))


class FakeBedrockClient:
    def __init__(self, delay=0.0, fail=False):
        self.delay, self.fail, self.kwargs = delay, fail, None

    def converse(self, **kwargs):
        self.kwargs = kwargs
        if self.delay:
            time.sleep(self.delay)
        if self.fail:
            raise RuntimeError("ThrottlingException")
        return {
            "output": {"message": {"content": [{"text": '{"a":'}, {"text": "1}"}]}},
            "usage": {"inputTokens": 9, "outputTokens": 3},
        }


def test_bedrock_converse_mapping():
    client = FakeBedrockClient()
    resp = asyncio.run(BedrockProvider(client=client).complete(REQ))
    assert resp.text == '{"a":1}'
    assert (resp.input_tokens, resp.output_tokens) == (9, 3)
    assert client.kwargs["system"] == [{"text": "sys"}]
    assert client.kwargs["messages"][0]["role"] == "user"


def test_bedrock_errors():
    with pytest.raises(LLMError):
        asyncio.run(BedrockProvider(client=FakeBedrockClient(fail=True)).complete(REQ))
    slow = CompletionRequest(messages=REQ.messages, timeout_s=0.05)
    with pytest.raises(LLMTimeoutError):
        asyncio.run(BedrockProvider(client=FakeBedrockClient(delay=0.3)).complete(slow))


def _client(handler):
    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


def test_anthropic_payload_and_headers():
    def handler(request):
        body = json.loads(request.content)
        assert request.headers["x-api-key"] == "k"
        assert body["system"] == "sys"
        assert all(m["role"] != "system" for m in body["messages"])
        return httpx.Response(
            200,
            json={
                "content": [{"type": "text", "text": "{}"}],
                "usage": {"input_tokens": 4, "output_tokens": 2},
            },
        )

    p = AnthropicProvider("m", "https://api.example", "k", client=_client(handler))
    resp = asyncio.run(p.complete(REQ))
    assert resp.text == "{}" and resp.output_tokens == 2


def test_openai_compatible_payload():
    def handler(request):
        body = json.loads(request.content)
        assert request.headers["authorization"] == "Bearer k"
        assert body["response_format"] == {"type": "json_object"}
        return httpx.Response(
            200,
            json={
                "choices": [{"message": {"content": "{}"}}],
                "usage": {"prompt_tokens": 7, "completion_tokens": 1},
            },
        )

    p = OpenAICompatibleProvider("m", "http://gw", "k", client=_client(handler))
    resp = asyncio.run(p.complete(REQ))
    assert resp.input_tokens == 7


def test_missing_key_rejected():
    with pytest.raises(LLMError):
        AnthropicProvider("m", "https://api.example", "")
