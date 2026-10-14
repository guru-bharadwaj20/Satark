"""/ask: answer an operator's question using PromQL and LogQL tool calls.

A small JSON tool protocol is used instead of provider-native tool calling so
it works the same on local Ollama models and hosted APIs. All tools are
read-only, bounded and their output is redacted by the LLM client.
"""

from __future__ import annotations

import json
import logging
import re
from dataclasses import dataclass, field

import httpx

from app.context import LokiClient
from app.llm import LLMClient, LLMError

log = logging.getLogger("satark.opsbot.ask")

MAX_STEPS = 4
MAX_QUERY_CHARS = 500
MAX_RESULT_CHARS = 3000

SYSTEM = """You are opsbot, a read-only assistant for the Satark Kubernetes platform.
Answer the operator's question using tools. Reply with exactly ONE JSON object per turn:

  {"tool": "promql", "query": "<PromQL instant query>"}
  {"tool": "logql", "query": "<LogQL query with a stream selector>", "minutes": <1-60>}
  {"answer": "<final answer for a phone screen, cite the queries you used>"}

Useful facts: application namespaces are satark, satark-data, satark-sandbox; metrics use the
prefix satark_ (e.g. satark_http_requests_total, satark_llm_tokens_total); Loki labels include
namespace, pod, container. Use at most 3 tool calls. Never invent numbers: only report what
tools returned. If tools fail, say so."""


class PrometheusClient:
    def __init__(self, base_url: str, client: httpx.AsyncClient | None = None) -> None:
        self._client = client or httpx.AsyncClient(base_url=base_url, timeout=15.0)

    async def query(self, promql: str) -> str:
        r = await self._client.get("/api/v1/query", params={"query": promql})
        data = r.json()
        if r.status_code != 200 or data.get("status") != "success":
            return f"error: {data.get('error', f'HTTP {r.status_code}')}"
        res = data["data"]
        if res["resultType"] in ("scalar", "string"):
            return str(res["result"][1])
        rows = []
        for s in res["result"][:30]:
            labels = ",".join(f'{k}="{v}"' for k, v in sorted(s["metric"].items()))
            value = s.get("value", [None, "?"])[1] if "value" in s else f"{len(s['values'])} pts"
            rows.append(f"{{{labels}}} {value}")
        more = len(res["result"]) - 30
        return "\n".join(rows) + (f"\n... {more} more series" if more > 0 else "") or "no data"


@dataclass
class AskResult:
    answer: str
    tool_calls: list[tuple[str, str]] = field(default_factory=list)
    tokens: int = 0


_SELECTOR = re.compile(r"\{[^}]*=[^}]*\}")


def _parse(text: str) -> dict:
    s = text.strip().removeprefix("```json").removeprefix("```").removesuffix("```").strip()
    try:
        obj = json.loads(s)
    except json.JSONDecodeError:
        start, end = s.find("{"), s.rfind("}")
        obj = json.loads(s[start : end + 1]) if 0 <= start < end else {"answer": text.strip()}
    return obj if isinstance(obj, dict) else {"answer": str(obj)}


class AskAgent:
    def __init__(self, llm: LLMClient, prometheus: PrometheusClient, loki: LokiClient) -> None:
        self.llm, self.prom, self.loki = llm, prometheus, loki

    async def _run_tool(self, call: dict) -> str:
        tool, query = call.get("tool"), str(call.get("query", "")).strip()
        if not query or len(query) > MAX_QUERY_CHARS:
            return "error: query missing or too long"
        try:
            if tool == "promql":
                out = await self.prom.query(query)
            elif tool == "logql":
                if not _SELECTOR.search(query):
                    return 'error: LogQL needs a stream selector like {namespace="satark"}'
                minutes = min(60, max(1, int(call.get("minutes", 15))))
                lines = await self.loki.query_range(query, minutes=minutes, limit=50)
                out = "\n".join(lines) or "no log lines"
            else:
                return f"error: unknown tool {tool!r}"
        except (httpx.HTTPError, ValueError, KeyError) as exc:
            return f"error: {type(exc).__name__}"
        return out[:MAX_RESULT_CHARS]

    async def ask(self, question: str) -> AskResult:
        result = AskResult("")
        transcript = f"QUESTION: {question[:1000]}"
        for step in range(MAX_STEPS):
            final = step == MAX_STEPS - 1
            prompt = transcript + ("\n\nYou must answer now." if final else "")
            try:
                reply = await self.llm.chat(SYSTEM, prompt, max_tokens=500, json_mode=True)
            except LLMError as exc:
                result.answer = f"LLM unavailable ({exc}); try again later."
                return result
            result.tokens += reply.total_tokens
            call = _parse(reply.text)
            if "answer" in call or final:
                result.answer = str(call.get("answer") or reply.text).strip()
                return result
            output = await self._run_tool(call)
            result.tool_calls.append((str(call.get("tool")), str(call.get("query"))))
            transcript += f"\n\nTOOL CALL {step + 1}: {json.dumps(call)}\nRESULT:\n{output}"
        result.answer = result.answer or "No answer."
        return result
