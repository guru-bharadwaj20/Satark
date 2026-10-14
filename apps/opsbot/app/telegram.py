"""Telegram Bot API client for the ops channel."""

from __future__ import annotations

import html
import logging

import httpx

log = logging.getLogger("satark.opsbot.telegram")
LIMIT = 4000  # Telegram's hard limit is 4096 characters per message


def escape(text: str) -> str:
    return html.escape(text, quote=False)


def split_message(text: str, limit: int = LIMIT) -> list[str]:
    parts: list[str] = []
    while len(text) > limit:
        cut = text.rfind("\n", 0, limit)
        cut = cut if cut > limit // 2 else limit
        parts.append(text[:cut])
        text = text[cut:].lstrip("\n")
    if text:
        parts.append(text)
    return parts


class TelegramClient:
    def __init__(
        self,
        token: str,
        client: httpx.AsyncClient | None = None,
        base_url: str = "https://api.telegram.org",
    ) -> None:
        if not token:
            raise ValueError("telegram bot token is not configured")
        self._url = f"{base_url}/bot{token}"
        self._client = client or httpx.AsyncClient(timeout=15.0)

    async def send(self, chat_id: str | int, text_html: str) -> None:
        """Send pre-escaped HTML, splitting long messages. Never logs the token."""
        for part in split_message(text_html):
            try:
                r = await self._client.post(
                    f"{self._url}/sendMessage",
                    json={
                        "chat_id": chat_id,
                        "text": part,
                        "parse_mode": "HTML",
                        "disable_web_page_preview": True,
                    },
                )
            except httpx.HTTPError as exc:
                raise RuntimeError(f"telegram send failed: {type(exc).__name__}") from None
            if r.status_code != 200:
                # Retry once as plain text in case of an HTML parse error.
                r = await self._client.post(
                    f"{self._url}/sendMessage", json={"chat_id": chat_id, "text": part}
                )
                if r.status_code != 200:
                    raise RuntimeError(f"telegram returned HTTP {r.status_code}")
