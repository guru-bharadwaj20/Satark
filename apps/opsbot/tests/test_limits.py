import asyncio
from datetime import UTC, datetime, timedelta

from test_diagnose_telegram import ALERT, FakeGatherer, FakeLLM, FakeRetriever, FakeTelegram

from app.diagnose import Diagnoser
from app.limits import Cooldown, DailyTokenBudget
from app.pipeline import AlertPipeline


def test_cooldown_per_key():
    now = [0.0]
    c = Cooldown(60, clock=lambda: now[0])
    assert c.try_acquire("a") and not c.try_acquire("a") and c.try_acquire("b")
    now[0] = 61
    assert c.try_acquire("a")


def test_budget_resets_daily():
    t = [datetime(2026, 10, 14, 23, 0, tzinfo=UTC)]
    b = DailyTokenBudget(1000, now=lambda: t[0])
    b.record(900)
    assert b.remaining == 100 and not b.allows(500)
    t[0] += timedelta(hours=2)
    assert b.remaining == 1000 and b.allows(500)


def make(budget_limit=100_000, cooldown=1800):
    tg, llm = FakeTelegram(), FakeLLM()
    p = AlertPipeline(
        Diagnoser(FakeGatherer(), FakeRetriever(), llm),
        tg,
        "c",
        Cooldown(cooldown),
        DailyTokenBudget(budget_limit),
    )
    return p, tg, llm


def test_repeat_alert_within_cooldown_is_skipped():
    p, tg, llm = make()
    asyncio.run(p(ALERT))
    asyncio.run(p(ALERT))
    assert len(tg.sent) == 1 and len(llm.prompts) == 1


def test_budget_exhausted_posts_context_only_and_one_notice():
    p, tg, llm = make(budget_limit=2500, cooldown=0)
    asyncio.run(p(ALERT))  # uses 380 tokens -> 2120 left, above the 2000 estimate
    asyncio.run(p(ALERT))  # 1740 left
    asyncio.run(p(ALERT))  # below estimate: no LLM call
    asyncio.run(p(ALERT))
    assert len(llm.prompts) == 2
    texts = [t for _, t in tg.sent]
    assert sum("budget reached; diagnoses are context-only" in t for t in texts) == 1
    assert "Daily LLM token budget reached - raw context only" in texts[-1]
