from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_healthz() -> None:
    assert client.get("/healthz").json() == {"status": "ok"}


def test_readyz() -> None:
    assert client.get("/readyz").status_code == 200


def test_metrics_exposes_satark_prefix() -> None:
    client.get("/healthz")
    body = client.get("/metrics").text
    assert "satark_text_analyzer_requests_total" in body
