import asyncio
import json

import httpx
from fastapi.testclient import TestClient
from test_diagnose_telegram import FakeTelegram

from app.ask import AskAgent, PrometheusClient
from app.commands import CommandRouter, ask_handler
from app.config import Settings
from app.context import LokiClient
from app.limits import DailyTokenBudget
from app.llm import LLMReply
from app.main import create_app


class ScriptedLLM:
    def __init__(self, replies):
        self.replies, self.prompts = list(replies), []

    async def chat(self, system, user, max_tokens=800, json_mode=False):
        self.prompts.append(user)
        return LLMReply(self.replies.pop(0), 50, 10)


def prom_handler(request):
    assert request.url.params["query"] == 'sum(rate(satark_http_requests_total{code=~"5.."}[5m]))'
    return httpx.Response(
        200,
        json={
            "status": "success",
            "data": {
                "resultType": "vector",
                "result": [{"metric": {"service": "url-intel"}, "value": [1, "0.42"]}],
            },
        },
    )


def loki_handler(request):
    return httpx.Response(200, json={"data": {"result": [{"values": [["1", "ERROR upstream"]]}]}})


def agent(replies):
    llm = ScriptedLLM(replies)
    prom = PrometheusClient(
        "http://p",
        client=httpx.AsyncClient(base_url="http://p", transport=httpx.MockTransport(prom_handler)),
    )
    loki = LokiClient(
        "http://l",
        client=httpx.AsyncClient(base_url="http://l", transport=httpx.MockTransport(loki_handler)),
    )
    return AskAgent(llm, prom, loki), llm


PROMQL = 'sum(rate(satark_http_requests_total{code=~"5.."}[5m]))'


def test_tool_loop_promql_then_logql_then_answer():
    a, llm = agent(
        [
            json.dumps({"tool": "promql", "query": PROMQL}),
            json.dumps({"tool": "logql", "query": '{namespace="satark", pod=~"url-intel.*"}'}),
            json.dumps({"answer": "url-intel has 0.42 errors/s; logs show upstream errors."}),
        ]
    )
    res = asyncio.run(a.ask("why are there 5xx?"))
    assert res.answer.startswith("url-intel has 0.42")
    assert [t for t, _ in res.tool_calls] == ["promql", "logql"]
    assert 'service="url-intel"} 0.42' in llm.prompts[1]
    assert "ERROR upstream" in llm.prompts[2]
    assert res.tokens == 180


def test_logql_without_selector_rejected_and_step_limit():
    a, llm = agent([json.dumps({"tool": "logql", "query": "error"})] * 3 + ["final words"])
    res = asyncio.run(a.ask("q"))
    assert "needs a stream selector" in llm.prompts[1]
    assert res.answer == "final words" and "must answer now" in llm.prompts[-1]


def router(allowed=(42,)):
    tg = FakeTelegram()
    budget = DailyTokenBudget(100_000)
    r = CommandRouter(tg, "-100", list(allowed), budget)
    a, _ = agent([json.dumps({"answer": "all good <ok>"})])
    r.register("/ask", ask_handler(a, budget))
    return r, tg, budget


def upd(text, chat=-100, user=42):
    return {"message": {"text": text, "chat": {"id": chat}, "from": {"id": user}}}


def test_router_ask_and_access_control():
    r, tg, budget = router()
    asyncio.run(r.handle_update(upd("/ask@satark_ops_bot how is it?")))
    assert "all good &lt;ok&gt;" in tg.sent[0][1] and budget.used == 60
    assert asyncio.run(r.handle_update(upd("/ask x", user=7))) is None
    assert asyncio.run(r.handle_update(upd("/ask x", chat=-5))) is None
    asyncio.run(r.handle_update(upd("/nope")))
    assert "Unknown command" in tg.sent[-1][1]


def test_telegram_webhook_requires_secret():
    r, tg, _ = router()
    app = create_app(None, Settings(webhook_token="a", telegram_webhook_secret="s3"), r)
    with TestClient(app) as c:
        assert c.post("/v1/telegram", json=upd("/help")).status_code == 401
        resp = c.post(
            "/v1/telegram", json=upd("/help"), headers={"X-Telegram-Bot-Api-Secret-Token": "s3"}
        )
        assert resp.json() == {"ok": True, "handled": True}
        for t in list(app.state.tasks):
            c.portal.call(asyncio.wait, [t])
    assert "opsbot commands" in tg.sent[0][1]
