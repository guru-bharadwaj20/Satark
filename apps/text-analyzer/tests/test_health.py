import pytest
from fastapi.testclient import TestClient

from app.main import app


@pytest.fixture(scope="module")
def client():
    with TestClient(app) as c:
        yield c


def test_healthz(client: TestClient) -> None:
    assert client.get("/healthz").json() == {"status": "ok"}


def test_readyz(client: TestClient) -> None:
    assert client.get("/readyz").status_code == 200


def test_metrics_exposes_satark_prefix(client: TestClient) -> None:
    client.get("/healthz")
    body = client.get("/metrics").text
    assert "satark_text_analyzer_requests_total" in body


def test_analyze_redacts_before_matching(client: TestClient) -> None:
    resp = client.post("/v1/analyze", json={"text": "Share OTP with 9876543210", "lang": "en"})
    assert resp.status_code == 200
    body = resp.json()
    assert "9876543210" not in body["redacted_text"]
    assert body["redactions"] == {"PHONE": 1}
    assert any(m["category"] == "otp" for m in body["matches"])
