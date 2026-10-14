"""Telegram command handling for the ops channel (/ask, /postmortem, /help)."""

from __future__ import annotations

import logging
from collections.abc import Awaitable, Callable

from app.limits import DailyTokenBudget
from app.telegram import escape

log = logging.getLogger("satark.opsbot.commands")

HELP = (
    "<b>opsbot commands</b>\n"
    "/ask &lt;question&gt; - answer using PromQL and Loki (read-only)\n"
    "/help - this message"
)

Handler = Callable[[str], Awaitable[str]]


class CommandRouter:
    """Routes Telegram updates. Only allow-listed users in the ops chat are served."""

    def __init__(
        self, telegram, ops_chat_id: str, allowed_user_ids: list[int], budget: DailyTokenBudget
    ) -> None:
        self.telegram, self.ops_chat_id = telegram, str(ops_chat_id)
        self.allowed = set(allowed_user_ids)
        self.budget = budget
        self.handlers: dict[str, Handler] = {}
        self.help = HELP

    def register(self, command: str, handler: Handler, help_line: str = "") -> None:
        self.handlers[command] = handler
        if help_line:
            self.help += "\n" + help_line

    async def handle_update(self, update: dict) -> str | None:
        msg = update.get("message") or update.get("channel_post") or {}
        text = (msg.get("text") or "").strip()
        chat_id = str(msg.get("chat", {}).get("id", ""))
        user_id = msg.get("from", {}).get("id")
        if not text.startswith("/"):
            return None
        if chat_id != self.ops_chat_id or (self.allowed and user_id not in self.allowed):
            log.warning("ignoring command from chat=%s user=%s", chat_id, user_id)
            return None
        cmd, _, arg = text.partition(" ")
        cmd = cmd.split("@", 1)[0].lower()
        if cmd in ("/help", "/start"):
            reply = self.help
        elif cmd in self.handlers:
            if not self.budget.allows():
                reply = "Daily LLM token budget reached; try again after 00:00 UTC."
            else:
                try:
                    reply = await self.handlers[cmd](arg.strip())
                except Exception:
                    log.exception("command %s failed", cmd)
                    reply = "Sorry, that command failed. Check opsbot logs."
        else:
            reply = f"Unknown command {escape(cmd)}. Try /help."
        await self.telegram.send(chat_id, reply)
        return reply


def ask_handler(agent, budget: DailyTokenBudget) -> Handler:
    async def run(question: str) -> str:
        if not question:
            return "Usage: /ask &lt;question&gt;, e.g. /ask why is url-intel restarting?"
        res = await agent.ask(question)
        budget.record(res.tokens)
        lines = [escape(res.answer)]
        if res.tool_calls:
            lines.append("")
            lines += [f"<code>{escape(t)}: {escape(q)}</code>" for t, q in res.tool_calls]
        lines.append(f"\n<i>{res.tokens} tokens; read-only queries.</i>")
        return "\n".join(lines)

    return run
