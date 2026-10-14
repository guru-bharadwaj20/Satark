import asyncio

from fastapi.testclient import TestClient

from app.config import Settings
from app.main import create_app

PAYLOAD = {
    "version": "4",
    "status": "firing",
    "receiver": "opsbot",
    "alerts": [
        {
            "status": "firing",
            "labels": {
                "alertname": "KubePodCrashLooping",
                "namespace": "satark",
                "pod": "url-intel-abc",
            },
            "annotations": {"summary": "crashloop"},
            "fingerprint": "f1",
        },
        {"status": "resolved", "labels": {"alertname": "X", "namespace": "satark"}},
        {"status": "firing", "labels": {"alertname": "Y", "namespace": "kube-system"}},
    ],
}


def make(handler=None, **kw):
    s = Settings(webhook_token="t0k", **kw)
    return create_app(handler, s)


def test_health_and_metrics():
    with TestClient(make()) as c:
        assert c.get("/healthz").json() == {"status": "ok"}
        assert c.get("/readyz").status_code == 200
        assert "satark_opsbot_alerts_received_total" in c.get("/metrics").text


def test_rejects_bad_token():
    with TestClient(make()) as c:
        assert c.post("/v1/alertmanager", json=PAYLOAD).status_code == 401
        r = c.post("/v1/alertmanager", json=PAYLOAD, headers={"Authorization": "Bearer nope"})
        assert r.status_code == 401


def test_unconfigured_token_fails_closed():
    with TestClient(create_app(None, Settings())) as c:
        r = c.post("/v1/alertmanager", json=PAYLOAD, headers={"Authorization": "Bearer "})
        assert r.status_code == 503


def test_only_firing_alerts_in_watched_namespaces_are_handled():
    seen = []

    async def handler(alert):
        seen.append(alert.name)

    app = make(handler)
    with TestClient(app) as c:
        r = c.post("/v1/alertmanager", json=PAYLOAD, headers={"Authorization": "Bearer t0k"})
        assert r.status_code == 202 and r.json() == {"accepted": 1, "received": 3}
        c.portal.call(app.state.queue.queue.join)
    assert seen == ["KubePodCrashLooping"]


def test_invalid_payload_422():
    with TestClient(make()) as c:
        r = c.post(
            "/v1/alertmanager", json={"alerts": "x"}, headers={"Authorization": "Bearer t0k"}
        )
        assert r.status_code == 422


def test_queue_full_drops():
    async def slow(alert):
        await asyncio.sleep(10)

    app = make(slow, queue_size=1)
    with TestClient(app) as c:
        many = {**PAYLOAD, "alerts": [PAYLOAD["alerts"][0]] * 5}
        r = c.post("/v1/alertmanager", json=many, headers={"Authorization": "Bearer t0k"})
        assert r.json()["accepted"] <= 2
