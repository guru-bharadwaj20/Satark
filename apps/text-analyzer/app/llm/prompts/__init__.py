"""Versioned prompt templates and message builders.

Changing a template file must bump PROMPT_VERSION: the version is part of the
response cache key and is recorded in the evaluation report.
"""

from __future__ import annotations

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
    hits = ", ".join(sorted(set(rule_hits or []))) or "none"
    system = _template("classify_v1").format(
        rule_hits=hits, language=language, language_name=LANGUAGE_NAMES[language]
    )
    user = f"<message>\n{redacted_text}\n</message>"
    return (Message("system", system), Message("user", user))
