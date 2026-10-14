"""Detect password, OTP and card-number input fields on a page.

Field attributes are collected in the browser by ``COLLECT_FIELDS_JS`` and
classified here in plain Python so the heuristics are unit-testable.
"""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass

from .report import Forms

log = logging.getLogger(__name__)

# Runs in every frame. Returns visible-ish <input> elements and their hints.
COLLECT_FIELDS_JS = r"""
() => {
  const labelFor = (el) => {
    const parts = [];
    if (el.labels) for (const l of el.labels) parts.push(l.innerText || "");
    const aria = el.getAttribute("aria-label"); if (aria) parts.push(aria);
    const by = el.getAttribute("aria-labelledby");
    if (by) for (const id of by.split(/\s+/)) {
      const n = document.getElementById(id); if (n) parts.push(n.innerText || "");
    }
    return parts.join(" ").slice(0, 200);
  };
  return Array.from(document.querySelectorAll("input")).slice(0, 200).map((el) => ({
    type: (el.getAttribute("type") || "text").toLowerCase(),
    name: el.getAttribute("name") || "",
    id: el.id || "",
    placeholder: el.getAttribute("placeholder") || "",
    autocomplete: (el.getAttribute("autocomplete") || "").toLowerCase(),
    inputmode: (el.getAttribute("inputmode") || "").toLowerCase(),
    maxlength: el.maxLength > 0 ? el.maxLength : null,
    label: labelFor(el),
    hidden: el.type === "hidden" || el.offsetParent === null && el.getClientRects().length === 0,
  }));
}
"""

OTP_WORDS = re.compile(
    r"\b(otp|one[\s_-]?time|verification[\s_-]?code|passcode|mpin|m[\s_-]?pin|upi[\s_-]?pin"
    r"|sms[\s_-]?code|auth[\s_-]?code|security[\s_-]?code)\b|ओटीपी|ಒಟಿಪಿ",
    re.IGNORECASE,
)
PASSWORD_WORDS = re.compile(r"\b(password|passwd|pwd)\b|पासवर्ड|ಪಾಸ್‌ವರ್ಡ್", re.IGNORECASE)
CARD_WORDS = re.compile(
    r"\b(card[\s_-]?(no|num|number)|cc[\s_-]?(num|number)|ccnum|debit[\s_-]?card|credit[\s_-]?card"
    r"|pan[\s_-]?number)\b|कार्ड\s*नंबर|ಕಾರ್ಡ್",
    re.IGNORECASE,
)
CVV_WORDS = re.compile(r"\b(cvv|cvc|csc|card[\s_-]?verification)\b", re.IGNORECASE)


@dataclass(frozen=True)
class Field:
    type: str = "text"
    name: str = ""
    id: str = ""
    placeholder: str = ""
    autocomplete: str = ""
    inputmode: str = ""
    maxlength: int | None = None
    label: str = ""
    hidden: bool = False

    @classmethod
    def from_dict(cls, d: dict) -> Field:
        known = {k: d[k] for k in cls.__dataclass_fields__ if k in d}
        return cls(**known)

    @property
    def text(self) -> str:
        # Split camelCase/underscores so "cardNumber" matches "card number".
        raw = " ".join([self.name, self.id, self.placeholder, self.label])
        raw = re.sub(r"([a-z])([A-Z])", r"\1 \2", raw)
        return raw.replace("_", " ").replace("-", " ")


def is_password(f: Field) -> bool:
    if f.type == "password" and not is_otp(f):
        return True
    return f.autocomplete in ("current-password", "new-password") or (
        f.type == "text" and bool(PASSWORD_WORDS.search(f.text))
    )


def is_otp(f: Field) -> bool:
    if f.autocomplete == "one-time-code":
        return True
    if OTP_WORDS.search(f.text):
        return True
    # Split OTP boxes: several single-character numeric inputs.
    return f.maxlength == 1 and f.inputmode == "numeric"


def is_card_number(f: Field) -> bool:
    if f.autocomplete in ("cc-number", "cc-csc"):
        return True
    if CARD_WORDS.search(f.text) or CVV_WORDS.search(f.text):
        return True
    return f.maxlength in (16, 19) and f.inputmode == "numeric" and "card" in f.text.lower()


def classify(raw_fields: list[dict]) -> Forms:
    forms = Forms()
    fields = [Field.from_dict(d) for d in raw_fields]
    visible = [f for f in fields if not f.hidden and f.type not in ("hidden", "submit", "button")]
    split_otp_boxes = sum(1 for f in visible if f.maxlength == 1) >= 4
    for f in visible:
        forms.otp = forms.otp or is_otp(f)
        forms.password = forms.password or is_password(f)
        forms.card_number = forms.card_number or is_card_number(f)
    forms.otp = forms.otp or split_otp_boxes
    return forms


async def detect(page) -> Forms:
    """Collect fields from every frame of ``page`` and classify them."""
    collected: list[dict] = []
    for frame in page.frames:
        try:
            collected.extend(await frame.evaluate(COLLECT_FIELDS_JS))
        except Exception as exc:  # detached or cross-origin frames are expected
            log.debug("skipping frame %s: %s", frame.url, exc)
    return classify(collected)
