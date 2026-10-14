"""Redact secrets, tokens and personal data before text leaves the cluster.

Applied to every prompt by app.llm.LLMClient, so callers cannot forget it.
Patterns are ordered from most to least specific.
"""

from __future__ import annotations

import re
from collections import Counter

_PATTERNS: list[tuple[str, re.Pattern[str], str]] = [
    (
        "private_key",
        re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----", re.S),
        "[REDACTED_PRIVATE_KEY]",
    ),
    (
        "jwt",
        re.compile(r"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b"),
        "[REDACTED_JWT]",
    ),
    ("bearer", re.compile(r"(?i)\b(bearer|token)\s+[A-Za-z0-9._~+/=-]{16,}"), r"\1 [REDACTED]"),
    ("aws_access_key", re.compile(r"\b(AKIA|ASIA)[A-Z0-9]{16}\b"), "[REDACTED_AWS_KEY]"),
    (
        "github_token",
        re.compile(r"\b(gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{50,})\b"),
        "[REDACTED_GITHUB_TOKEN]",
    ),
    ("slack_token", re.compile(r"\bxox[abprs]-[A-Za-z0-9-]{10,}\b"), "[REDACTED_SLACK_TOKEN]"),
    (
        "telegram_bot_token",
        re.compile(r"\b\d{8,10}:[A-Za-z0-9_-]{35}\b"),
        "[REDACTED_TELEGRAM_TOKEN]",
    ),
    ("anthropic_key", re.compile(r"\bsk-(ant-)?[A-Za-z0-9_-]{20,}\b"), "[REDACTED_API_KEY]"),
    (
        "url_credentials",
        re.compile(r"(?i)\b([a-z][a-z0-9+.-]*://)[^\s:/@]+:[^\s@/]+@"),
        r"\1[REDACTED]@",
    ),
    (
        "kv_secret",
        re.compile(
            r"(?i)\b([\w.-]*(?:password|passwd|pwd|secret|api[_-]?key|access[_-]?key|token|"
            r"client[_-]?secret|private[_-]?key)[\w.-]*)(\"?\s*[:=]\s*\"?)([^\s\"',;}]{4,})"
        ),
        r"\1\2[REDACTED]",
    ),
    (
        "k8s_secret_data",
        re.compile(r"(?m)^(\s+[\w.-]+:\s+)[A-Za-z0-9+/]{40,}={0,2}\s*$"),
        r"\1[REDACTED_BASE64]",
    ),
    ("email", re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b"), "[EMAIL]"),
    ("upi", re.compile(r"\b[A-Za-z0-9._-]{2,}@[A-Za-z]{2,}\b"), "[UPI]"),
    ("phone_in", re.compile(r"(?<![\d.])(?:\+91[\s-]?|0)?[6-9]\d{9}(?![\d.])"), "[PHONE]"),
    (
        "ipv4_public",
        re.compile(
            r"\b(?!10\.|127\.|192\.168\.|172\.(?:1[6-9]|2\d|3[01])\.)(?:\d{1,3}\.){3}\d{1,3}\b"
        ),
        "[PUBLIC_IP]",
    ),
]


def redact(text: str) -> tuple[str, Counter]:
    """Return the redacted text and a count of what was redacted, by kind."""
    counts: Counter = Counter()
    for kind, rx, repl in _PATTERNS:
        text, n = rx.subn(repl, text)
        if n:
            counts[kind] += n
    return text, counts


def redact_text(text: str) -> str:
    return redact(text)[0]
