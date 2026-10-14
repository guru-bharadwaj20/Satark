import json

import pytest

from app.llm.prompts import build_classification_messages
from app.llm.schema import Category, SchemaError, parse_classification

GOOD = {
    "category": "utility_bill",
    "is_scam": True,
    "confidence": 0.9,
    "reasons": ["Deadline of tonight", "Personal mobile number [PHONE]"],
    "explanation": "This looks like a scam. Check your bill on the official app.",
    "language": "en",
}


def test_valid_output_parses():
    r = parse_classification(json.dumps(GOOD))
    assert r.category is Category.UTILITY_BILL and r.is_scam


def test_fenced_and_prefixed_json():
    r = parse_classification("Sure!\n```json\n" + json.dumps(GOOD) + "\n```")
    assert r.confidence == 0.9


@pytest.mark.parametrize(
    "patch",
    [
        {"category": "not-a-category"},
        {"confidence": 1.5},
        {"category": "legitimate"},  # contradicts is_scam=True
        {"is_scam": False},  # scam category but not scam
        {"explanation": "This message is 100% safe."},
        {"reasons": ["The sender is a scammer"]},
        {"reasons": ["a", "b", "c", "d", "e"]},
        {"extra_field": 1},
        {"language": "fr"},
    ],
)
def test_invalid_outputs_rejected(patch):
    with pytest.raises(SchemaError):
        parse_classification(json.dumps({**GOOD, **patch}))


def test_garbage_rejected():
    for text in ["", "no json here", "[1,2]", "{broken"]:
        with pytest.raises(SchemaError):
            parse_classification(text)


def test_prompt_contains_rules_and_language():
    system, user = build_classification_messages("Pay now [UPI]", ["urgency", "otp"], "hi")
    assert system.role == "system" and "otp, urgency" in system.content
    assert "Hindi" in system.content and '"language": "hi"' in system.content
    assert user.content.startswith("<message>") and "Pay now [UPI]" in user.content
