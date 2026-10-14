"""Deterministic keyword baseline.

It stands in for the rule engine when no LLM is available (for example in CI
without secrets) and is used for LLM fallbacks during evaluation, mirroring
the production "rules-only" path. It is intentionally simple.
"""

from __future__ import annotations

import re

# Order matters: the first matching category wins.
PATTERNS: list[tuple[str, str]] = [
    (
        "remote_access",
        r"anydesk|teamviewer|rustdesk|screen-share|\.apk\b|\bapk\b|"
        r"ಸ್ಥಾಪಿಸಿ|इंस्टॉल|download karein|ಆ್ಯಪ್ ಅನ್ನು .* ಡೌನ್‌ಲೋಡ್",
    ),
    (
        "impersonation_authority",
        r"police|cbi\b|trai\b|court|summons|digital arrest|income tax|"
        r"पुलिस|अरेस्ट|ಪೊಲೀಸ್|ನ್ಯಾಯಾಲಯ",
    ),
    (
        "otp_pin",
        r"(share|forward|tell|reply|bata|bhej).{0,40}(otp|code|cvv)|"
        r"(otp|cvv).{0,30}(bhej|bata|ಹೇಳಿ|बताएं)|upi pin|enter .*pin|pin daal|"
        r"approve the collect|ಹೇಳಿ",
    ),
    (
        "lottery_prize",
        r"lottery|lucky (draw|winner)|kbc|won\b|winner|gift voucher|"
        r"लॉटरी|लकी ड्रॉ|jeeta|ಲಕ್ಕಿ|ಬಹುಮಾನ",
    ),
    (
        "task_job",
        r"part[- ]time|task|work from home|data entry|vip level|ghar baithe|"
        r"पार्ट टाइम|ಅರೆಕಾಲಿಕ|ಮನೆಯಿಂದಲೇ",
    ),
    ("investment", r"stock|crypto|usdt|ipo|returns?|intraday|profit|स्टॉक|ಷೇರು|ಹೂಡಿ"),
    ("delivery_parcel", r"parcel|package|shipment|customs|redelivery|पार्सल|ಪಾರ್ಸೆಲ್|ಕಸ್ಟಮ್ಸ್"),
    ("utility_bill", r"(disconnect|power cut|cut today|connection will).*|bijli|बिजली|ವಿದ್ಯುತ್"),
    (
        "kyc_banking",
        r"kyc|pan is not linked|reward points|account (will be )?(blocked|suspended|"
        r"on hold)|deactivated|card is blocked|रिवॉर्ड|ಡೆಬಿಟ್ ಕಾರ್ಡ್ ಬ್ಲಾಕ್",
    ),
    (
        "other_scam",
        r"new number|naya number|ಹೊಸ ಸಂಖ್ಯೆ|instant loan|लोन|ಸಾಲ|subsidy|"
        r"army officer|update card",
    ),
]
_COMPILED = [(c, re.compile(p, re.I)) for c, p in PATTERNS]

# Benign phrasing that should suppress weak matches.
_BENIGN = re.compile(
    r"do not share|never asks for your otp|किसी के साथ साझा न|ಯಾರೊಂದಿಗೂ ಹಂಚಿಕೊಳ್ಳಬೇಡಿ|"
    r"official app|आधिकारिक ऐप|ಅಧಿಕೃತ ಆ್ಯಪ್|not you\?|नहीं थे\?|ನೀವಲ್ಲವೇ|"
    r"has been shipped|was delivered|kyc is complete|no fees are charged|"
    r"booking is confirmed|बुकिंग कन्फर्म",
    re.I,
)
_HAS_LINK_OR_PHONE = re.compile(r"https?://|\d{2}X{4,}\d{2}|\*\*\*@")


def predict(text: str) -> str:
    benign = bool(_BENIGN.search(text)) and not _HAS_LINK_OR_PHONE.search(text)
    if benign:
        return "legitimate"
    for cat, rx in _COMPILED:
        if rx.search(text):
            return cat
    return "legitimate"
