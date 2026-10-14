"""Parse UPI deep links (``upi://pay?...``) per the NPCI linking spec.

Only the fields Satark explains to users are extracted: ``pa`` (payee
address), ``pn`` (payee name), ``am`` (amount), ``cu`` (currency) and
``tn`` (transaction note). Payee addresses are masked before they leave
this module (``xy***@examplebank``).
"""

from __future__ import annotations

import re
from dataclasses import asdict, dataclass, field
from decimal import Decimal, InvalidOperation
from typing import Any
from urllib.parse import parse_qs, urlsplit

_VPA_RE = re.compile(r"^[\w.\-]{2,256}@[A-Za-z][A-Za-z0-9]{1,63}$")
_MAX_FIELD = 256


class UPIParseError(ValueError):
    """Raised when the text is not a UPI deep link."""


@dataclass
class UPIIntent:
    payee_address: str | None = None
    payee_name: str | None = None
    amount: str | None = None
    currency: str | None = None
    note: str | None = None
    action: str = "pay"
    direction: str = "send"
    issues: list[str] = field(default_factory=list)

    @property
    def summary(self) -> str:
        to = self.payee_address or "an unknown account"
        if self.payee_name:
            to = f"{to} ({self.payee_name})"
        if self.amount:
            return f"This SENDS {format_inr(self.amount)} FROM your account to {to}"
        return f"This SENDS money FROM your account to {to}"

    def as_dict(self) -> dict[str, Any]:
        d = asdict(self)
        d["summary"] = self.summary
        return d


def mask_vpa(vpa: str) -> str:
    """Mask a UPI ID, keeping the first two characters and the PSP handle."""
    handle, _, psp = vpa.partition("@")
    return f"{handle[:2]}***@{psp}" if psp else f"{handle[:2]}***"


def format_inr(amount: str) -> str:
    """Format an amount with Indian digit grouping, e.g. 1499900 -> ₹14,99,900."""
    value = Decimal(amount)
    whole, _, frac = f"{value:.2f}".partition(".")
    sign = "-" if whole.startswith("-") else ""
    whole = whole.lstrip("-")
    if len(whole) > 3:
        head, tail = whole[:-3], whole[-3:]
        groups = []
        while len(head) > 2:
            groups.insert(0, head[-2:])
            head = head[:-2]
        if head:
            groups.insert(0, head)
        whole = ",".join(groups) + "," + tail
    text = f"₹{sign}{whole}"
    return text if frac == "00" else f"{text}.{frac}"


def _first(params: dict[str, list[str]], key: str) -> str | None:
    values = params.get(key)
    if not values:
        return None
    v = values[0].strip()
    return v[:_MAX_FIELD] or None


def parse_upi(uri: str) -> UPIIntent:
    parts = urlsplit(uri.strip())
    if parts.scheme.lower() != "upi":
        raise UPIParseError("not a upi:// link")
    params = {k.lower(): v for k, v in parse_qs(parts.query, keep_blank_values=False).items()}
    intent = UPIIntent(action=(parts.netloc or parts.path.strip("/") or "pay").lower())

    pa = _first(params, "pa")
    if pa is None:
        intent.issues.append("missing_payee")
    else:
        if not _VPA_RE.match(pa):
            intent.issues.append("invalid_payee")
        intent.payee_address = mask_vpa(pa)

    intent.payee_name = _first(params, "pn")

    am = _first(params, "am")
    if am is not None:
        try:
            value = Decimal(am)
            if value <= 0 or not value.is_finite():
                raise InvalidOperation
            intent.amount = f"{value.quantize(Decimal('0.01'))}"
        except InvalidOperation:
            intent.issues.append("invalid_amount")

    cu = _first(params, "cu")
    intent.currency = cu.upper() if cu else None
    if intent.currency and intent.currency != "INR":
        intent.issues.append("non_inr_currency")

    intent.note = _first(params, "tn")
    if intent.action not in ("pay", "mandate"):
        intent.issues.append("unknown_action")
    return intent
