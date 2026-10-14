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


def luhn_valid(number: str) -> bool:
    """Return True if the digits of ``number`` pass the Luhn checksum."""
    digits = _digits(number)
    if not digits:
        return False
    total = 0
    for i, ch in enumerate(reversed(digits)):
        d = int(ch)
        if i % 2 == 1:
            d *= 2
            if d > 9:
                d -= 9
        total += d
    return total % 10 == 0


# 13-19 digit payment card numbers, optionally grouped by spaces or dashes.
_CARD = re.compile(r"(?<![\w])\d(?:[\s-]?\d){12,18}(?![\w])")


def _is_card(value: str) -> bool:
    digits = _digits(value)
    return 13 <= len(digits) <= 19 and len(set(digits)) > 1 and luhn_valid(digits)


CARD = Detector("CARD", _CARD, _is_card)

# Verhoeff dihedral-group tables (multiplication, permutation, inverse).
_VERHOEFF_D = (
    (0, 1, 2, 3, 4, 5, 6, 7, 8, 9),
    (1, 2, 3, 4, 0, 6, 7, 8, 9, 5),
    (2, 3, 4, 0, 1, 7, 8, 9, 5, 6),
    (3, 4, 0, 1, 2, 8, 9, 5, 6, 7),
    (4, 0, 1, 2, 3, 9, 5, 6, 7, 8),
    (5, 9, 8, 7, 6, 0, 4, 3, 2, 1),
    (6, 5, 9, 8, 7, 1, 0, 4, 3, 2),
    (7, 6, 5, 9, 8, 2, 1, 0, 4, 3),
    (8, 7, 6, 5, 9, 3, 2, 1, 0, 4),
    (9, 8, 7, 6, 5, 4, 3, 2, 1, 0),
)
_VERHOEFF_P = (
    (0, 1, 2, 3, 4, 5, 6, 7, 8, 9),
    (1, 5, 7, 6, 2, 8, 3, 0, 9, 4),
    (5, 8, 0, 3, 7, 9, 6, 1, 4, 2),
    (8, 9, 1, 6, 0, 4, 3, 5, 2, 7),
    (9, 4, 5, 3, 1, 2, 6, 8, 7, 0),
    (4, 2, 8, 6, 5, 7, 3, 9, 0, 1),
    (2, 7, 9, 3, 8, 0, 6, 4, 1, 5),
    (7, 0, 4, 6, 9, 1, 3, 2, 5, 8),
)


def verhoeff_valid(number: str) -> bool:
    """Return True if the digits of ``number`` pass the Verhoeff checksum."""
    digits = _digits(number)
    if not digits:
        return False
    c = 0
    for i, ch in enumerate(reversed(digits)):
        c = _VERHOEFF_D[c][_VERHOEFF_P[i % 8][int(ch)]]
    return c == 0


# Aadhaar: 12 digits, first digit 2-9, commonly written as 4-4-4 groups.
_AADHAAR = re.compile(r"(?<![\w])[2-9]\d{3}[\s-]?\d{4}[\s-]?\d{4}(?![\w])")


def _is_aadhaar(value: str) -> bool:
    digits = _digits(value)
    return len(digits) == 12 and verhoeff_valid(digits)


AADHAAR = Detector("AADHAAR", _AADHAAR, _is_aadhaar)

DETECTORS: tuple[Detector, ...] = (CARD, AADHAAR, PHONE, LANDLINE)


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
