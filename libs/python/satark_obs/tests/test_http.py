from fastapi import FastAPI, HTTPException
from fastapi.testclient import TestClient

from satark_obs import HTTPMetrics, PrometheusMiddleware, mount_endpoints


def make_app(checks=None):
    metrics = HTTPMetrics("text-analyzer")
    app = FastAPI()

    @app.post("/v1/analyze/{lang}")
    async def analyze(lang: str):
        if lang == "xx":
            raise HTTPException(status_code=422)
        return {"ok": True}

    app.add_middleware(PrometheusMiddleware, metrics=metrics)
    mount_endpoints(app, metrics, checks)
    return app, metrics


def sample(metrics, name, **labels):
    return metrics.registry.get_sample_value(name, labels)


def test_red_metrics_use_route_template():
    app, metrics = make_app()
    client = TestClient(app)
    client.post("/v1/analyze/en")
    client.post("/v1/analyze/hi")
    client.post("/v1/analyze/xx")

    labels = dict(service="text-analyzer", method="POST", route="/v1/analyze/{lang}")
    assert sample(metrics, "satark_http_requests_total", code="200", **labels) == 2
    assert sample(metrics, "satark_http_requests_total", code="422", **labels) == 1
    assert sample(metrics, "satark_http_request_duration_seconds_count", **labels) == 3
    assert sample(metrics, "satark_http_requests_in_flight", service="text-analyzer") == 0


def test_unknown_path_is_bucketed():
    app, metrics = make_app()
    TestClient(app).get("/nope/123")
    assert sample(
        metrics, "satark_http_requests_total", service="text-analyzer", method="GET", route="unmatched", code="404"
    ) == 1


def test_metrics_endpoint_and_health():
    app, _ = make_app()
    client = TestClient(app)
    client.get("/healthz")
    assert client.get("/healthz").json() == {"status": "ok"}
    body = client.get("/metrics").text
    assert 'satark_http_requests_total{code="200",method="GET",route="/healthz",service="text-analyzer"}' in body


def test_readyz_reports_failures():
    async def ok():
        return None

    async def down():
        raise ConnectionError("valkey")

    app, _ = make_app({"postgres": ok, "valkey": down})
    resp = TestClient(app).get("/readyz")
    assert resp.status_code == 503
    assert resp.json()["failed"] == {"valkey": "ConnectionError"}
