import asyncio

import httpx

from app.alerts import Alert
from app.context import ContextGatherer, KubeClient, LokiClient

POD = {
    "metadata": {
        "name": "url-intel-7d9f-abc",
        "ownerReferences": [{"kind": "ReplicaSet", "name": "url-intel-7d9f"}],
    },
    "spec": {"nodeName": "worker-1"},
    "status": {
        "phase": "Running",
        "conditions": [{"type": "Ready", "status": "False", "reason": "ContainersNotReady"}],
        "containerStatuses": [
            {
                "name": "app",
                "ready": False,
                "restartCount": 7,
                "image": "ghcr.io/x/url-intel:1.2.0",
                "state": {"waiting": {"reason": "CrashLoopBackOff"}},
                "lastState": {"terminated": {"reason": "Error", "exitCode": 1}},
            }
        ],
    },
}
EVENTS = {
    "items": [
        {
            "type": "Warning",
            "reason": "BackOff",
            "message": "Back-off restarting failed container",
            "involvedObject": {"kind": "Pod", "name": "url-intel-7d9f-abc"},
            "lastTimestamp": "2026-10-14T10:00:00Z",
            "count": 12,
        },
        {
            "type": "Normal",
            "reason": "Pulled",
            "message": "other pod",
            "involvedObject": {"kind": "Pod", "name": "gateway-api-1"},
            "lastTimestamp": "2026-10-14T09:00:00Z",
        },
    ]
}
RS = {
    "items": [
        {
            "metadata": {
                "name": f"url-intel-{h}",
                "creationTimestamp": f"2026-10-1{i}T00:00:00Z",
                "annotations": {"deployment.kubernetes.io/revision": str(i)},
                "ownerReferences": [{"name": "url-intel"}],
            },
            "spec": {"template": {"spec": {"containers": [{"image": f"url-intel:1.{i}.0"}]}}},
            "status": {"replicas": 1, "readyReplicas": 0 if i == 4 else 1},
        }
        for i, h in [(2, "aa"), (3, "bb"), (4, "cc")]
    ]
}


def kube_handler(request: httpx.Request) -> httpx.Response:
    p = request.url.path
    if p.endswith("/pods/url-intel-7d9f-abc"):
        return httpx.Response(200, json=POD)
    if p.endswith("/events"):
        return httpx.Response(200, json=EVENTS)
    if p.endswith("/replicasets"):
        return httpx.Response(200, json=RS)
    return httpx.Response(404, json={})


def loki_handler(request: httpx.Request) -> httpx.Response:
    assert 'pod="url-intel-7d9f-abc"' in request.url.params["query"]
    return httpx.Response(
        200, json={"data": {"result": [{"values": [["2", "panic: nil map"], ["1", "starting"]]}]}}
    )


def gatherer(loki=loki_handler):
    kube = KubeClient(
        "https://k8s",
        client=httpx.AsyncClient(
            base_url="https://k8s", transport=httpx.MockTransport(kube_handler)
        ),
    )
    lk = LokiClient(
        "http://loki",
        client=httpx.AsyncClient(base_url="http://loki", transport=httpx.MockTransport(loki)),
    )
    return ContextGatherer(kube, lk)


ALERT = Alert(
    status="firing",
    labels={
        "alertname": "KubePodCrashLooping",
        "namespace": "satark",
        "pod": "url-intel-7d9f-abc",
        "severity": "warning",
    },
)


def test_gathers_all_sections():
    ctx = asyncio.run(gatherer().gather(ALERT))
    text = ctx.render()
    assert "CrashLoopBackOff" in text and "restarts=7" in text and "exitCode=1" in text
    assert "BackOff" in ctx.sections["events"] and "gateway-api" not in ctx.sections["events"]
    rollouts = ctx.sections["recent rollouts"].splitlines()
    assert rollouts[0].startswith("revision 4") and "ready=0/1" in rollouts[0]
    assert ctx.sections["logs (last 15m)"] == "starting\npanic: nil map"
    assert not ctx.errors


def test_failing_source_is_reported_not_fatal():
    def broken(request):
        return httpx.Response(500)

    ctx = asyncio.run(gatherer(broken).gather(ALERT))
    assert "logs (last 15m)" in ctx.errors and "events" in ctx.sections
    assert "(unavailable)" in ctx.render()
