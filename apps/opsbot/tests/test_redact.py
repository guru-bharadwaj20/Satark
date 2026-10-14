import asyncio
import json

import httpx
import pytest

from app.llm import LLMClient
from app.redact import redact

# All values below are fake and only shaped like real credentials.
CASES = [
    ("Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", "abcdefghijklmnop", "bearer"),
    ("key AKIAABCDEFGHIJKLMNOP used", "AKIAABCDEFGHIJKLMNOP", "aws_access_key"),
    ("ghp_" + "a" * 36, "ghp_aaaa", "github_token"),
    ("bot 123456789:" + "A" * 35, "AAAAAAAAAA", "telegram_bot_token"),
    ("jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJlLWZha2U", "eyJhbGci", "jwt"),
    ("postgres://satark:hunter2pass@satark-pg-rw:5432/app", "hunter2pass", "url_credentials"),
    ("DB_PASSWORD=sup3rs3cret starting", "sup3rs3cret", "kv_secret"),
    ('{"api_key": "abcd1234efgh"}', "abcd1234efgh", "kv_secret"),
    (
        "-----BEGIN RSA PRIVATE KEY-----\nMIIfake\n-----END RSA PRIVATE KEY-----",
        "MIIfake",
        "private_key",
    ),
    ("sk-ant-api03-FAKEFAKEFAKEFAKEFAKE", "FAKEFAKE", "anthropic_key"),
    ("user called from 9876543210", "9876543210", "phone_in"),
    ("payer ab.cd@okexample", "ab.cd@okexample", "upi"),
    ("mail ops@example.com", "ops@example.com", "email"),
    ("client 203.0.113.9 connected", "203.0.113.9", "ipv4_public"),
]


@pytest.mark.parametrize("text,secret,kind", CASES)
def test_redacts(text, secret, kind):
    out, counts = redact(text)
    assert secret not in out
    assert counts[kind] >= 1


def test_keeps_useful_context():
    text = (
        "Back-off restarting failed container app in pod url-intel-7d9f-abc "
        "on 192.168.56.11 exit code 137 at 2026-10-14T10:00:00Z"
    )
    assert redact(text)[0] == text


def test_client_redacts_before_sending():
    sent = {}

    def handler(request):
        sent.update(json.loads(request.content))
        return httpx.Response(
            200, json={"message": {"content": "ok"}, "prompt_eval_count": 5, "eval_count": 2}
        )

    c = LLMClient(
        "ollama", "http://o", "m", client=httpx.AsyncClient(transport=httpx.MockTransport(handler))
    )
    reply = asyncio.run(c.chat("sys", "log: DB_PASSWORD=hunter22 Bearer " + "x" * 30))
    body = json.dumps(sent)
    assert "hunter22" not in body and "x" * 30 not in body
    assert reply.total_tokens == 7
