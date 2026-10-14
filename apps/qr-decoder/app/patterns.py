"""Risk patterns for decoded UPI QR codes.

The key fact: scanning a UPI QR and entering your PIN always SENDS money.
Nobody ever needs to scan a code to *receive* money, so any hint that a
QR is being presented as a way to receive money is a strong scam signal.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from decimal import Decimal

from app.upi import UPIIntent

# Wording that frames a payment QR as a way to receive money, in English,
# Hindi (Devanagari and romanised) and Kannada.
_RECEIVE_RE = re.compile(
    r"(?i)("
    r"\b(receive|receiving|get|collect|claim)\b.{0,25}\b(money|payment|amount|cash|refund|prize|reward)\b"
    r"|\b(refund|cashback|cash ?back|prize|reward|lottery|winning|bonus)\b"
    r"|\b(payment received|credit(ed)?)\b"
    r"|\bscan (to|and) (receive|get|claim)\b"
    r"|पैसे (पाने|प्राप्त)|रिफंड|कैशबैक|इनाम|प्राप्त करें"
    r"|ಹಣ ಪಡೆ|ರಿಫಂಡ್|ಕ್ಯಾಶ್‌ಬ್ಯಾಕ್|ಬಹುಮಾನ|ಸ್ವೀಕರಿಸಿ"
    r"|\b(paise milenge|paisa milega|paise paane)\b"
    r")"
)

HIGH_AMOUNT = Decimal("10000")


@dataclass(frozen=True)
class Flag:
    code: str
    weight: int
    message: str


def detect_flags(intent: UPIIntent, context: str | None = None) -> list[Flag]:
    """Return risk flags for a parsed UPI intent.

    ``context`` is optional text that accompanied the QR (a chat caption or
    the message the user forwarded with it).
    """
    flags: list[Flag] = [
        Flag(
            "qr.upi_sends_money",
            10,
            "Scanning this code and entering your UPI PIN sends money from your account",
        )
    ]
    haystack = " ".join(filter(None, (intent.note, intent.payee_name, context)))
    if haystack and _RECEIVE_RE.search(haystack):
        flags.append(
            Flag(
                "qr.receive_money",
                60,
                "This QR is presented as a way to receive money, but scanning a UPI QR "
                "always sends money. You never need to scan or enter a PIN to receive",
            )
        )
    if intent.amount and Decimal(intent.amount) >= HIGH_AMOUNT:
        flags.append(Flag("qr.high_amount", 15, "The code is pre-filled with a large amount"))
    if "invalid_payee" in intent.issues or "missing_payee" in intent.issues:
        flags.append(Flag("qr.malformed", 20, "The payment details in this code are malformed"))
    if "non_inr_currency" in intent.issues:
        flags.append(Flag("qr.foreign_currency", 15, "The code asks for a non-rupee payment"))
    return flags
