"""PII redaction.

Every piece of personal data is replaced with a typed placeholder such as
``[PHONE]`` before text leaves this service (rules, logs, LLM calls).

Detectors run in priority order. A span claimed by an earlier detector is
never re-claimed by a later one, so e.g. a 16-digit card number is not
also partially matched as a phone number.
"""

from __future__ import annotations

import re
from collections import Counter
from collections.abc import Callable
from dataclasses import dataclass, field

Validator = Callable[[str], bool]


@dataclass(frozen=True)
class Detector:
    kind: str
    pattern: re.Pattern[str]
    validator: Validator | None = None
    group: int = 0

    @property
    def placeholder(self) -> str:
        return f"[{self.kind}]"


@dataclass
class RedactionResult:
    text: str
    counts: Counter[str] = field(default_factory=Counter)

    @property
    def total(self) -> int:
        return sum(self.counts.values())


def _digits(value: str) -> str:
    return re.sub(r"\D", "", value)


# Indian mobile numbers: optional +91 / 91 / 0 prefix, then 10 digits starting
# 6-9, optionally split by spaces or dashes. Partially masked numbers such as
# 98XXXXXX21 are treated as phone numbers too.
_PHONE = re.compile(
    r"(?<![\w+])"
    r"(?:\+?91[\s-]?|0)?"
    r"[6-9][0-9Xx*](?:[\s-]?[0-9Xx*]){8}"
    r"(?![\w])"
)
# Landlines / toll-free with STD code, e.g. 080-2345 6789, 1800-123-4567.
_LANDLINE = re.compile(r"(?<![\w])(?:0\d{2,4}|1800)[\s-]\d{3,4}[\s-]?\d{3,4}(?![\w])")


def _is_phone(value: str) -> bool:
    core = re.sub(r"[^0-9Xx*]", "", value)
    if core.startswith("91") and len(core) == 12:
        core = core[2:]
    elif core.startswith("0") and len(core) == 11:
        core = core[1:]
    return len(core) == 10 and core[0] in "6789"


PHONE = Detector("PHONE", _PHONE, _is_phone)
LANDLINE = Detector("PHONE", _LANDLINE)

DETECTORS: tuple[Detector, ...] = (PHONE, LANDLINE)


def redact(text: str, detectors: tuple[Detector, ...] = DETECTORS) -> RedactionResult:
    """Replace personal data in ``text`` with typed placeholders."""
    claimed: list[tuple[int, int, str]] = []

    def overlaps(start: int, end: int) -> bool:
        return any(start < e and s < end for s, e, _ in claimed)

    for det in detectors:
        for m in det.pattern.finditer(text):
            start, end = m.span(det.group)
            value = m.group(det.group)
            if overlaps(start, end):
                continue
            if det.validator is not None and not det.validator(value):
                continue
            claimed.append((start, end, det.kind))

    claimed.sort()
    out: list[str] = []
    counts: Counter[str] = Counter()
    pos = 0
    for start, end, kind in claimed:
        out.append(text[pos:start])
        out.append(f"[{kind}]")
        counts[kind] += 1
        pos = end
    out.append(text[pos:])
    return RedactionResult(text="".join(out), counts=counts)
