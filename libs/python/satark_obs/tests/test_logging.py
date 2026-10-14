import io
import json
import logging

from fastapi import FastAPI
from fastapi.testclient import TestClient

from satark_obs.logging import REDACTED, AccessLogMiddleware, configure_logging, mask_pii, trace_id_var


def lines(buf):
    return [json.loads(line) for line in buf.getvalue().splitlines() if line]


def test_json_fields_and_redaction(monkeypatch):
    monkeypatch.setenv("SATARK_VERSION", "0.4.0")
    monkeypatch.setenv("SATARK_ENV", "dev")
    buf = io.StringIO()
    log = configure_logging("text-analyzer", stream=buf)
    token = trace_id_var.set("t-1")
    try:
        log.warning("scored", extra={"text": "you won", "score": 0.91, "note": "ping 9876543210"})
    finally:
        trace_id_var.reset(token)
    (rec,) = lines(buf)
    assert rec["service"] == "text-analyzer"
    assert rec["version"] == "0.4.0" and rec["env"] == "dev"
    assert rec["level"] == "WARN" and rec["msg"] == "scored"
    assert rec["trace_id"] == "t-1"
    assert rec["text"] == REDACTED
    assert rec["score"] == 0.91
    assert rec["note"] == "ping [PHONE]"


def test_mask_pii():
    assert mask_pii("pay abcd@ybl now, call +91-98765-43210") == "pay [UPI] now, call [PHONE]"


def test_access_log_middleware():
    buf = io.StringIO()
    configure_logging("qr-decoder", stream=buf)
    app = FastAPI()

    @app.get("/v1/decode")
    async def decode():
        logging.getLogger("qr").info("inside")
        return {"ok": True}

    app.add_middleware(AccessLogMiddleware)
    resp = TestClient(app).get("/v1/decode?data=secret", headers={"X-Request-ID": "abc"})
    assert resp.headers["x-request-id"] == "abc"
    # Ignore the test client's own (httpx) log lines.
    inside, access = [r for r in lines(buf) if r["logger"] in ("qr", "access")]
    assert inside["trace_id"] == "abc"
    assert access["msg"] == "request" and access["path"] == "/v1/decode" and access["status"] == 200
    assert "secret" not in json.dumps(access)
