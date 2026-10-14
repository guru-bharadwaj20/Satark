"""qr-decoder tests using the sample images in tests/fixtures."""

from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from app.decode import DecodeError, decode_image
from app.main import app
from app.patterns import detect_flags
from app.upi import UPIParseError, format_inr, mask_vpa, parse_upi

FIXTURES = Path(__file__).parent / "fixtures"
client = TestClient(app)


def fixture(name: str) -> bytes:
    return (FIXTURES / name).read_bytes()


def post(name: str, context: str | None = None):
    data = {"context": context} if context else None
    return client.post("/v1/decode", files={"image": (name, fixture(name))}, data=data)


# --- decoding --------------------------------------------------------------


@pytest.mark.parametrize(
    "name", ["upi_merchant.png", "upi_inverted.png", "upi_receive_photo.jpg", "url.png"]
)
def test_decode_samples(name: str) -> None:
    codes = decode_image(fixture(name))
    assert len(codes) == 1
    assert codes[0].format == "QRCode"


def test_blank_image_has_no_codes() -> None:
    assert decode_image(fixture("blank.png")) == []


def test_non_image_rejected() -> None:
    with pytest.raises(DecodeError):
        decode_image(b"definitely not an image")


# --- UPI parsing -----------------------------------------------------------------


def test_parse_upi_fields() -> None:
    i = parse_upi(
        "upi://pay?pa=xyz123@examplebank&pn=Refund%20Desk&am=14999&cu=INR&tn=Scan%20to%20receive"
    )
    assert i.payee_address == "xy***@examplebank"
    assert i.payee_name == "Refund Desk"
    assert i.amount == "14999.00"
    assert i.currency == "INR"
    assert i.note == "Scan to receive"
    assert i.direction == "send"
    assert i.issues == []
    assert i.summary == ("This SENDS ₹14,999 FROM your account to xy***@examplebank (Refund Desk)")


def test_parse_upi_case_insensitive_keys() -> None:
    i = parse_upi("UPI://pay?PA=shop01@examplebank&AM=5.5")
    assert i.payee_address == "sh***@examplebank"
    assert i.amount == "5.50"


@pytest.mark.parametrize(
    ("uri", "issue"),
    [
        ("upi://pay?am=10", "missing_payee"),
        ("upi://pay?pa=not-a-vpa", "invalid_payee"),
        ("upi://pay?pa=ab@examplebank&am=-5", "invalid_amount"),
        ("upi://pay?pa=ab@examplebank&am=abc", "invalid_amount"),
        ("upi://pay?pa=ab@examplebank&cu=USD", "non_inr_currency"),
    ],
)
def test_parse_upi_issues(uri: str, issue: str) -> None:
    assert issue in parse_upi(uri).issues


def test_parse_non_upi() -> None:
    with pytest.raises(UPIParseError):
        parse_upi("https://example.org")


@pytest.mark.parametrize(
    ("amount", "expected"),
    [("5", "₹5"), ("14999", "₹14,999"), ("1499900", "₹14,99,900"), ("10.5", "₹10.50")],
)
def test_format_inr(amount: str, expected: str) -> None:
    assert format_inr(amount) == expected


def test_mask_vpa() -> None:
    assert mask_vpa("9876543210@ybl") == "98***@ybl"


# --- patterns ------------------------------------------------------------------


def codes(intent_uri: str, context: str | None = None) -> set[str]:
    return {f.code for f in detect_flags(parse_upi(intent_uri), context)}


def test_merchant_qr_only_informational() -> None:
    assert codes("upi://pay?pa=teashop01@examplebank&pn=Tea%20Stall") == {"qr.upi_sends_money"}


def test_receive_money_in_note() -> None:
    assert "qr.receive_money" in codes("upi://pay?pa=ab@examplebank&tn=refund%20for%20order")


@pytest.mark.parametrize(
    "context",
    [
        "Scan this QR to receive your payment",
        "स्कैन करें और पैसे पाने के लिए PIN डालें",
        "ರಿಫಂಡ್ ಪಡೆಯಲು ಸ್ಕ್ಯಾನ್ ಮಾಡಿ",
        "scan karo, paise milenge",
    ],
)
def test_receive_money_in_context(context: str) -> None:
    assert "qr.receive_money" in codes("upi://pay?pa=ab@examplebank", context)


def test_high_amount_and_malformed() -> None:
    assert {"qr.high_amount", "qr.malformed", "qr.foreign_currency"} <= codes(
        "upi://pay?pa=bad&am=50000&cu=USD"
    )


# --- HTTP API ------------------------------------------------------------------------


def test_health_endpoints() -> None:
    assert client.get("/healthz").json() == {"status": "ok"}
    assert client.get("/readyz").status_code == 200
    assert "satark_qr_decoder_requests_total" in client.get("/metrics").text


def test_api_receive_scam() -> None:
    resp = post("upi_receive_scam.png")
    assert resp.status_code == 200
    [code] = resp.json()["codes"]
    assert code["kind"] == "upi"
    assert code["text"] is None  # raw link holds an unmasked UPI ID
    assert code["upi"]["payee_address"] == "xy***@examplebank"
    assert "SENDS ₹14,999" in code["upi"]["summary"]
    assert {"qr.receive_money", "qr.high_amount"} <= {f["code"] for f in code["flags"]}


def test_api_context_field() -> None:
    [code] = post("upi_merchant.png", context="Scan to receive cashback").json()["codes"]
    assert "qr.receive_money" in {f["code"] for f in code["flags"]}


@pytest.mark.parametrize(("name", "kind"), [("url.png", "url"), ("plain_text.png", "text")])
def test_api_non_upi(name: str, kind: str) -> None:
    [code] = post(name).json()["codes"]
    assert code["kind"] == kind
    assert code["text"]
    assert code["flags"] == []


def test_api_blank() -> None:
    assert post("blank.png").json() == {"codes": []}


def test_api_rejects_garbage() -> None:
    resp = client.post("/v1/decode", files={"image": ("x.png", b"nope")})
    assert resp.status_code == 400


def test_api_rejects_empty() -> None:
    resp = client.post("/v1/decode", files={"image": ("x.png", b"")})
    assert resp.status_code == 400
