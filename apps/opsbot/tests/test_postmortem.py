import asyncio

import httpx
import pytest
from test_diagnose_telegram import ALERT, FakeGatherer, FakeLLM, FakeRetriever, FakeTelegram

from app.commands import postmortem_handler
from app.diagnose import Diagnoser
from app.limits import DailyTokenBudget
from app.llm import LLMError, LLMReply
from app.pipeline import AlertPipeline
from app.postmortem import IncidentHistory, PostmortemGenerator, alert_timeline, parse_args

NOW = 1_760_436_000.0  # 2025-10-14 10:00 UTC


@pytest.mark.parametrize(
    "arg,expected",
    [
        ("", (None, 6, "")),
        ("KubePodCrashLooping 3h bad image", ("KubePodCrashLooping", 3, "bad image")),
        ("12h", (None, 12, "")),
        ("The deploy broke it", (None, 6, "The deploy broke it")),
        ("SatarkLLMBudgetExceeded 200h", ("SatarkLLMBudgetExceeded", 6, "200h")),
        ("99h", (None, 72, "")),
    ],
)
def test_parse_args(arg, expected):
    assert parse_args(arg) == expected


def prom(handler):
    return httpx.AsyncClient(base_url="http://p", transport=httpx.MockTransport(handler))


def series_handler(request):
    assert 'alertname="KubePodCrashLooping"' in request.url.params["query"]
    start = NOW - 3600
    vals = [[start + i * 60, "1"] for i in range(10)] + [
        [start + 1800 + i * 60, "1"] for i in range(5)
    ]
    return httpx.Response(
        200,
        json={
            "data": {
                "result": [
                    {
                        "metric": {
                            "alertname": "KubePodCrashLooping",
                            "namespace": "satark",
                            "pod": "url-intel-1",
                            "severity": "critical",
                        },
                        "values": vals,
                    }
                ]
            }
        },
    )


def test_timeline_splits_intervals():
    lines = asyncio.run(alert_timeline(prom(series_handler), "KubePodCrashLooping", 1, NOW))
    assert len(lines) == 2
    assert lines[0].startswith("2025-10-14 09:00 -> 2025-10-14 09:09 KubePodCrashLooping")
    assert "satark/url-intel-1" in lines[0]


class CapturingLLM:
    def __init__(self, fail=False):
        self.fail, self.user = fail, ""

    async def chat(self, system, user, max_tokens=800, json_mode=False):
        self.user = user
        if self.fail:
            raise LLMError("down")
        assert "blameless" in system.lower()
        return LLMReply("# Postmortem: url-intel crashloop\n## Summary\nTBD", 400, 200)


def test_draft_uses_timeline_history_and_notes():
    hist = IncidentHistory()
    hist.add("KubePodCrashLooping", "satark", "critical", "Likely cause: bad image", at=NOW - 600)
    hist.add("Other", "satark", "warning", "unrelated", at=NOW - 600)
    llm = CapturingLLM()
    gen = PostmortemGenerator(llm, prom(series_handler), hist)
    text, tokens = asyncio.run(gen.draft("KubePodCrashLooping 1h rolled back at 09:40", now=NOW))
    assert text.startswith("# Postmortem") and tokens == 600
    assert "Likely cause: bad image" in llm.user and "unrelated" not in llm.user
    assert "rolled back at 09:40" in llm.user and "09:00 -> " in llm.user


def test_draft_without_llm_returns_skeleton_with_facts():
    gen = PostmortemGenerator(
        CapturingLLM(fail=True), prom(lambda r: httpx.Response(503)), IncidentHistory()
    )
    text, tokens = asyncio.run(gen.draft("", now=NOW))
    assert tokens == 0 and "## Action items" in text and "Prometheus unavailable" in text


def test_pipeline_records_history_and_handler_formats():
    tg = FakeTelegram()
    p = AlertPipeline(Diagnoser(FakeGatherer(), FakeRetriever(), FakeLLM()), tg, "c")
    asyncio.run(p(ALERT))
    assert p.history.entries[-1].alert == "KubePodCrashLooping"
    budget = DailyTokenBudget(10_000)
    gen = PostmortemGenerator(CapturingLLM(), prom(series_handler), p.history)
    out = asyncio.run(postmortem_handler(gen, budget)("KubePodCrashLooping"))
    assert "<pre># Postmortem" in out and budget.used == 600
