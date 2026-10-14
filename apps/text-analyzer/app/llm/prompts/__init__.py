"""Versioned prompt templates and message builders.

Changing a template file must bump PROMPT_VERSION: the version is part of the
response cache key and is recorded in the evaluation report.
"""

from __future__ import annotations

import re
import unicodedata
from functools import cache
from pathlib import Path

from app.llm.provider import Message

PROMPT_VERSION = "classify-v1"
_DIR = Path(__file__).resolve().parent

LANGUAGE_NAMES = {
    "en": "English",
    "hi": "Hindi (Devanagari script)",
    "kn": "Kannada (Kannada script)",
}


MAX_MESSAGE_CHARS = 2000

_TAG = re.compile(r"<\s*/?\s*message\s*>", re.I)
_RULE_ID = re.compile(r"[^a-z0-9_.-]")
# Common prompt-injection phrasings (English, Hinglish, Hindi). Used as a risk
# signal only; the prompt itself already treats the message as data.
_INJECTION = re.compile(
    r"ignore (all |any )?(the )?(previous|above|prior|earlier) (instructions|rules|prompt)"
    r"|disregard (the |your )?(instructions|rules|system prompt)"
    r"|you are now|act as (an? )?(admin|developer|system)|new instructions\s*:"
    r"|system prompt|\bjailbreak\b|developer mode"
    r"|(classify|mark|label|treat) (this|it) as (safe|legitimate|not (a )?scam)"
    r"|is_scam\"?\s*[:=]\s*false|\"category\"\s*:"
    r"|pichle (sabhi )?nirdesh (ko )?bhool|पिछले (सभी )?निर्देश"
    r"|ಹಿಂದಿನ ಸೂಚನೆ",
    re.I,
)


def sanitize_message(text: str) -> str:
    """Neutralise delimiter spoofing and control characters, and cap length."""
    text = unicodedata.normalize("NFKC", text)
    text = "".join(ch for ch in text if ch in "\n\t" or unicodedata.category(ch)[0] != "C")
    text = _TAG.sub("[tag]", text)
    if len(text) > MAX_MESSAGE_CHARS:
        text = text[:MAX_MESSAGE_CHARS] + " [truncated]"
    return text


def injection_suspected(text: str) -> bool:
    return bool(_INJECTION.search(unicodedata.normalize("NFKC", text)))


def _clean_rule_ids(rule_hits: list[str] | None) -> list[str]:
    out = {_RULE_ID.sub("", r.lower())[:48] for r in (rule_hits or [])}
    return sorted(r for r in out if r)


@cache
def _template(name: str) -> str:
    return (_DIR / f"{name}.txt").read_text(encoding="utf-8")


def build_classification_messages(
    redacted_text: str, rule_hits: list[str] | None = None, language: str = "en"
) -> tuple[Message, ...]:
    """Build the system + user messages for one classification call.

    ``redacted_text`` must already have passed PII redaction.
    """
    if language not in LANGUAGE_NAMES:
        language = "en"
    hits = ", ".join(_clean_rule_ids(rule_hits)) or "none"
    system = _template("classify_v1").format(
        rule_hits=hits, language=language, language_name=LANGUAGE_NAMES[language]
    )
    user = f"<message>\n{sanitize_message(redacted_text)}\n</message>"
    return (Message("system", system), Message("user", user))
