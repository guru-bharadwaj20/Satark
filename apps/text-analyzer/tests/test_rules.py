"""Rule engine and rule-set tests."""

from pathlib import Path

import pytest

from app.redact import redact
from app.rules import RuleEngine, RuleError, load_file

REPO_RULES = Path(__file__).resolve().parents[3] / "rules"


@pytest.fixture(scope="module")
def engine() -> RuleEngine:
    return RuleEngine.from_dir(REPO_RULES)


def ids(engine: RuleEngine, text: str, lang: str = "en") -> set[str]:
    return {m.id for m in engine.evaluate(redact(text).text, lang)}


def categories(engine: RuleEngine, text: str) -> set[str]:
    return {m.category for m in engine.evaluate(redact(text).text)}


# --- engine ------------------------------------------------------------------


def write(tmp_path: Path, body: str, name: str = "en.yaml") -> Path:
    p = tmp_path / name
    p.write_text(body, encoding="utf-8")
    return p


def test_load_minimal(tmp_path: Path) -> None:
    p = write(
        tmp_path,
        "version: 1\nlang: en\nrules:\n"
        "  - id: a.b\n    weight: 10\n    patterns: ['foo']\n    reason: r\n",
    )
    rules = load_file(p)
    assert rules[0].category == "a"
    assert rules[0].reason("hi") == "r"


@pytest.mark.parametrize(
    "rule",
    [
        "  - id: Bad\n    weight: 10\n    patterns: ['x']\n    reason: r\n",
        "  - id: a.b\n    weight: 0\n    patterns: ['x']\n    reason: r\n",
        "  - id: a.b\n    weight: 10\n    patterns: []\n    reason: r\n",
        "  - id: a.b\n    weight: 10\n    patterns: ['(']\n    reason: r\n",
        "  - id: a.b\n    weight: 10\n    patterns: ['x']\n",
    ],
)
def test_invalid_rules_rejected(tmp_path: Path, rule: str) -> None:
    p = write(tmp_path, "version: 1\nlang: en\nrules:\n" + rule)
    with pytest.raises(RuleError):
        load_file(p)


def test_wrong_version_rejected(tmp_path: Path) -> None:
    with pytest.raises(RuleError):
        load_file(write(tmp_path, "version: 2\nrules: []\n"))


def test_duplicate_ids_rejected(tmp_path: Path) -> None:
    rule = "  - id: a.b\n    weight: 10\n    patterns: ['x']\n    reason: r\n"
    write(tmp_path, "version: 1\nlang: en\nrules:\n" + rule, "en.yaml")
    write(tmp_path, "version: 1\nlang: hi\nrules:\n" + rule, "hi.yaml")
    with pytest.raises(RuleError, match="duplicate"):
        RuleEngine.from_dir(tmp_path)


def test_requires_all(tmp_path: Path) -> None:
    write(
        tmp_path,
        "version: 1\nlang: en\nrules:\n"
        "  - id: a.b\n    weight: 10\n    patterns: ['electricity']\n"
        "    requires: ['disconnect']\n    reason: r\n",
    )
    e = RuleEngine.from_dir(tmp_path)
    assert e.evaluate("electricity bill paid") == []
    assert len(e.evaluate("electricity will be disconnected")) == 1


def test_sorted_by_weight(engine: RuleEngine) -> None:
    matches = engine.evaluate("Share your OTP immediately")
    weights = [m.weight for m in matches]
    assert weights == sorted(weights, reverse=True)


def test_reason_localized(engine: RuleEngine) -> None:
    [m] = [m for m in engine.evaluate("share OTP", "hi") if m.id == "otp.share"]
    assert "OTP" in m.reason and "माँगता" in m.reason


# --- repository rule sets -------------------------------------------------------


def test_all_rule_files_load(engine: RuleEngine) -> None:
    langs = {r.source for r in engine.rules}
    assert langs == {"en.yaml", "hi.yaml", "kn.yaml"}
    for r in engine.rules:
        assert set(r.reasons) >= {"en", "hi", "kn"}, r.id
        for text in r.reasons.values():
            assert "100%" not in text
            assert "scammer" not in text.lower()


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        (
            "Dear Customer, your electricity connection will be disconnected tonight at 9:30 PM "
            "as your bill was not updated. Contact our electricity officer 98XXXXXX21",
            {"utility", "urgency"},
        ),
        ("Your bank account will be blocked today. Update your KYC here: bit.ly/xyz", {"kyc"}),
        ("Please install AnyDesk so we can process your refund", {"remote_access"}),
        ("Download the app from this link: http://example.test/bank.apk", {"remote_access"}),
        ("Share the OTP sent to your phone to verify", {"otp"}),
        ("Enter your UPI PIN to receive the cashback", {"otp"}),
        ("Congratulations! You have won a lucky draw of Rs 25,00,000", {"lottery"}),
        ("Like YouTube videos and earn Rs 3,000 per day. Part-time job", {"task_job"}),
    ],
)
def test_english_scams(engine: RuleEngine, text: str, expected: set[str]) -> None:
    assert expected <= categories(engine, text)


@pytest.mark.parametrize(
    "text",
    [
        "Hi, are we meeting for lunch tomorrow?",
        "Your electricity bill of Rs 1,240 has been paid. Thank you.",
        "Your order has been shipped and will arrive on Friday.",
    ],
)
def test_benign_messages(engine: RuleEngine, text: str) -> None:
    assert engine.evaluate(redact(text).text) == []


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        (
            "प्रिय ग्राहक, आपका बिजली का कनेक्शन आज रात काट दिया जाएगा। "
            "तुरंत अधिकारी 9876543210 पर कॉल करें",
            {"utility.disconnection_hi", "utility.call_officer_hi", "urgency.deadline_hi"},
        ),
        ("अपना OTP बताएं", {"otp.share_hi"}),
        ("बधाई हो, आपने लॉटरी जीती है", {"lottery.prize_hi"}),
        ("घर बैठे रोज़ 3000 कमाएं", {"task_job.earn_daily_hi"}),
    ],
)
def test_hindi_scams(engine: RuleEngine, text: str, expected: set[str]) -> None:
    assert expected <= ids(engine, text, "hi")


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        (
            "ನಿಮ್ಮ ವಿದ್ಯುತ್ ಸಂಪರ್ಕ ಇಂದು ರಾತ್ರಿ ಕಡಿತಗೊಳ್ಳುತ್ತದೆ. ಅಧಿಕಾರಿ 9876543210 ಗೆ ಕರೆ ಮಾಡಿ",
            {"utility.disconnection_kn", "utility.call_officer_kn", "urgency.deadline_kn"},
        ),
        ("ನಿಮ್ಮ ಒಟಿಪಿ ತಿಳಿಸಿ", {"otp.share_kn"}),
        ("ನಿಮ್ಮ ಕೆವೈಸಿ ಅಪ್‌ಡೇಟ್ ಮಾಡಿ", {"kyc.update_kn"}),
    ],
)
def test_kannada_scams(engine: RuleEngine, text: str, expected: set[str]) -> None:
    assert expected <= ids(engine, text, "kn")


def test_rules_never_see_raw_pii(engine: RuleEngine) -> None:
    redacted = redact("Contact officer 9876543210").text
    assert "9876543210" not in redacted
    assert "utility.call_officer" in {m.id for m in engine.evaluate(redacted)}
