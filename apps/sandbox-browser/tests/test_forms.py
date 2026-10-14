import pytest

from app.forms import classify


def test_empty_page():
    f = classify([])
    assert not (f.password or f.otp or f.card_number)


def test_password_field():
    assert classify([{"type": "password", "name": "pass"}]).password


def test_password_hidden_ignored():
    assert not classify([{"type": "password", "hidden": True}]).password


@pytest.mark.parametrize(
    "field",
    [
        {"type": "text", "autocomplete": "one-time-code"},
        {"type": "tel", "name": "otp"},
        {"type": "text", "placeholder": "Enter 6 digit OTP"},
        {"type": "password", "name": "mpin"},
        {"type": "text", "label": "UPI PIN"},
        {"type": "text", "placeholder": "ओटीपी दर्ज करें"},
    ],
)
def test_otp_fields(field):
    forms = classify([field])
    assert forms.otp
    if field["type"] == "password":
        assert not forms.password  # an MPIN box is an OTP/PIN, not a password


def test_split_otp_boxes():
    boxes = [{"type": "text", "maxlength": 1} for _ in range(6)]
    assert classify(boxes).otp


@pytest.mark.parametrize(
    "field",
    [
        {"type": "text", "autocomplete": "cc-number"},
        {"type": "tel", "name": "cardNumber"},
        {"type": "text", "placeholder": "Debit card number"},
        {"type": "text", "name": "cvv", "maxlength": 3},
    ],
)
def test_card_fields(field):
    assert classify([field]).card_number


def test_plain_search_box_is_clean():
    f = classify([{"type": "search", "name": "q", "placeholder": "Search"}])
    assert not (f.password or f.otp or f.card_number)
