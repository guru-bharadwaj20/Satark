import asyncio
import json

import httpx

from app.alerts import Alert
from app.context import IncidentContext
from app.diagnose import Diagnoser
from app.llm import LLMError, LLMReply
from app.pipeline import AlertPipeline
from app.rag.store import Hit
from app.telegram import TelegramClient, split_message

ALERT = Alert(
    status="firing",
    labels={
        "alertname": "KubePodCrashLooping",
        "namespace": "satark",
        "pod": "url-intel-1",
        "severity": "critical",
    },
)


class FakeGatherer:
    async def gather(self, alert):
        return IncidentContext(alert, {"pod status": "state=waiting reason=CrashLoopBackOff <x>"})


class FakeRetriever:
    async def for_alert(self, alert, text):
        assert "CrashLoopBackOff" in text
        return [Hit("RB-001#1", "RB-001", "RB-001 CrashLoop", "Fix", "roll back", 0.9)]


class FakeLLM:
    def __init__(self, fail=False):
        self.fail, self.prompts = fail, []

    async def chat(self, system, user, max_tokens=800, json_mode=False):
        self.prompts.append(user)
        if self.fail:
            raise LLMError("down")
        return LLMReply("Likely cause: bad image <tag>\nRunbooks: [1]", 300, 80)


class FakeTelegram:
    def __init__(self):
        self.sent = []

    async def send(self, chat_id, text):
        self.sent.append((chat_id, text))


def run_pipeline(llm):
    tg = FakeTelegram()
    p = AlertPipeline(Diagnoser(FakeGatherer(), FakeRetriever(), llm), tg, "-100123")
    asyncio.run(p(ALERT))
    return tg.sent


def test_diagnosis_posted_with_sources_and_escaping():
    llm = FakeLLM()
    [(chat, text)] = run_pipeline(llm)
    assert chat == "-100123"
    assert "[1] RB-001 CrashLoop - Fix" in llm.prompts[0]
    assert "<b>KubePodCrashLooping</b>" in text and "&lt;tag&gt;" in text
    assert "380 tokens" in text and "verify before acting" in text


def test_llm_outage_still_posts_context():
    [(_, text)] = run_pipeline(FakeLLM(fail=True))
    assert "LLM unavailable" in text and "CrashLoopBackOff" in text and "RB-001" in text


def test_split_long_messages():
    parts = split_message("line\n" * 2000, limit=1000)
    assert all(len(p) <= 1000 for p in parts) and "".join(parts).count("line") == 2000


def test_telegram_client_sends_html_and_falls_back_to_plain():
    calls = []

    def handler(request):
        body = json.loads(request.content)
        calls.append(body)
        assert "/bot123:abc/sendMessage" in request.url.path
        return httpx.Response(400 if body.get("parse_mode") else 200, json={})

    tg = TelegramClient("123:abc", client=httpx.AsyncClient(transport=httpx.MockTransport(handler)))
    asyncio.run(tg.send("-1", "<b>x</b>"))
    assert calls[0]["parse_mode"] == "HTML" and "parse_mode" not in calls[1]
