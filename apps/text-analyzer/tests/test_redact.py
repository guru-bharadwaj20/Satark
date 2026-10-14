"""Redaction tests. All fixtures are synthetic; none belong to real people."""

import pytest

from app.redact import luhn_valid, redact, verhoeff_valid

# Synthetic values that pass their checksums.
VALID_CARD = "4111 1111 1111 1111"  # standard Visa test number
VALID_AADHAAR = "2341 2341 2346"  # Verhoeff-valid, not issued
INVALID_AADHAAR = "2341 2341 2345"


@pytest.mark.parametrize(
    "text",
    [
        "call 9876543210 now",
        "call +91 98765 43210 now",
        "call +91-9876543210 now",
        "call 09876543210 now",
        "call 98XXXXXX21 now",
        "call 98765-43210 now",
        "call 080-2345 6789 now",
    ],
)
def test_phone_redacted(text: str) -> None:
    result = redact(text)
    assert result.text == "call [PHONE] now"
    assert result.counts["PHONE"] == 1


@pytest.mark.parametrize("text", ["ref 1234567890", "order 5555555", "Rs. 14,999 on 12/03/2024"])
def test_non_phone_numbers_survive(text: str) -> None:
    assert redact(text).text == text


def test_luhn() -> None:
    assert luhn_valid("4111111111111111")
    assert luhn_valid("5500 0000 0000 0004")
    assert not luhn_valid("4111111111111112")
    assert not luhn_valid("")


def test_card_redacted_only_when_luhn_valid() -> None:
    assert redact(f"card {VALID_CARD} exp").text == "card [CARD] exp"
    assert redact("card 4111-1111-1111-1111").text == "card [CARD]"
    assert redact("card 4111 1111 1111 1112").counts["CARD"] == 0


def test_card_with_same_digits_not_redacted() -> None:
    assert redact("0000000000000000").counts["CARD"] == 0


def test_verhoeff() -> None:
    assert verhoeff_valid("2363")
    assert not verhoeff_valid("2364")
    assert verhoeff_valid(VALID_AADHAAR)
    assert not verhoeff_valid(INVALID_AADHAAR)


def test_aadhaar_redacted_only_when_verhoeff_valid() -> None:
    assert redact(f"aadhaar {VALID_AADHAAR}").text == "aadhaar [AADHAAR]"
    assert redact(f"aadhaar {VALID_AADHAAR.replace(' ', '')}").text == "aadhaar [AADHAAR]"
    assert redact(f"aadhaar {INVALID_AADHAAR}").counts["AADHAAR"] == 0


def test_pan() -> None:
    assert redact("PAN ABCPE1234F linked").text == "PAN [PAN] linked"
    assert redact("pan abcpe1234f").text == "pan [PAN]"
    # 4th character must be a valid holder type.
    assert redact("code ABCXE1234F").counts["PAN"] == 0


def test_email_before_upi() -> None:
    result = redact("mail a.b+x@example.co.in or pay xy***@examplebank")
    assert result.text == "mail [EMAIL] or pay [UPI]"
    assert result.counts == {"EMAIL": 1, "UPI": 1}


@pytest.mark.parametrize("vpa", ["ravi.k@okaxis", "9876543210@ybl", "shop-12@paytm"])
def test_upi(vpa: str) -> None:
    assert redact(f"pay {vpa} now").text == "pay [UPI] now"


@pytest.mark.parametrize(
    "text",
    [
        "A/c No. 1234 5678 9012 credited",
        "account number: 123456789012345 credited",
        "acct 001234567890 credited",
        "खाता संख्या 123456789012 credited",
    ],
)
def test_account_with_keyword(text: str) -> None:
    assert "[ACCOUNT]" in redact(text).text
    assert redact(text).text.endswith(" credited")


def test_short_masked_account_kept() -> None:
    assert redact("your a/c XX1234 is blocked").text == "your a/c XX1234 is blocked"


def test_multiple_kinds() -> None:
    text = f"Card {VALID_CARD}, call 9876543210, mail x@example.com"
    result = redact(text)
    assert result.text == "Card [CARD], call [PHONE], mail [EMAIL]"
    assert result.total == 3


def test_unicode_text_untouched() -> None:
    text = "आपका बिजली का कनेक्शन आज रात काट दिया जाएगा"
    assert redact(text).text == text
