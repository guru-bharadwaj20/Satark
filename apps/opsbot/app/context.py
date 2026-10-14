"""Gather incident context for an alert: pod status, events, rollouts, logs.

All calls are read-only (see deploy/rbac.yaml). Every section is collected
independently; a failing source is reported in the context instead of
aborting the diagnosis.
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass, field
from pathlib import Path

import httpx

from app.alerts import Alert

log = logging.getLogger("satark.opsbot.context")

MAX_EVENTS = 20
MAX_LOG_LINES = 80
MAX_SECTION_CHARS = 4000


class KubeClient:
    def __init__(self, base_url: str, token: str = "", verify: str | bool = True, client=None):
        headers = {"Authorization": f"Bearer {token}"} if token else {}
        self._client = client or httpx.AsyncClient(
            base_url=base_url, headers=headers, verify=verify, timeout=10.0
        )

    @classmethod
    def in_cluster(cls, base_url: str, token_path: str, ca_path: str) -> KubeClient:
        token = Path(token_path).read_text().strip() if Path(token_path).exists() else ""
        verify: str | bool = ca_path if Path(ca_path).exists() else True
        return cls(base_url, token, verify)

    async def get(self, path: str, params: dict | None = None) -> dict:
        r = await self._client.get(path, params=params)
        r.raise_for_status()
        return r.json()

    async def get_text(self, path: str, params: dict | None = None) -> str:
        r = await self._client.get(path, params=params)
        r.raise_for_status()
        return r.text


class LokiClient:
    def __init__(self, base_url: str, client=None):
        self._client = client or httpx.AsyncClient(base_url=base_url, timeout=15.0)

    async def query_range(self, query: str, minutes: int = 15, limit: int = MAX_LOG_LINES) -> list:
        end = time.time_ns()
        start = end - minutes * 60 * 1_000_000_000
        r = await self._client.get(
            "/loki/api/v1/query_range",
            params={
                "query": query,
                "start": start,
                "end": end,
                "limit": limit,
                "direction": "backward",
            },
        )
        r.raise_for_status()
        lines: list[tuple[str, str]] = []
        for stream in r.json().get("data", {}).get("result", []):
            for ts, line in stream.get("values", []):
                lines.append((ts, line))
        lines.sort()
        return [line for _, line in lines[-limit:]]


@dataclass
class IncidentContext:
    alert: Alert
    sections: dict[str, str] = field(default_factory=dict)
    errors: dict[str, str] = field(default_factory=dict)

    def render(self) -> str:
        a = self.alert
        out = [
            f"ALERT {a.name} severity={a.severity} namespace={a.namespace or '-'}",
            f"labels: {', '.join(f'{k}={v}' for k, v in sorted(a.labels.items()))}",
        ]
        if a.annotations:
            out.append("annotations: " + "; ".join(f"{k}: {v}" for k, v in a.annotations.items()))
        for name, body in self.sections.items():
            out += ["", f"== {name} ==", body]
        for name, err in self.errors.items():
            out += ["", f"== {name} (unavailable) ==", err]
        return "\n".join(out)


def _clip(text: str, limit: int = MAX_SECTION_CHARS) -> str:
    return text if len(text) <= limit else text[: limit - 15] + "\n...[truncated]"


def summarize_pod(pod: dict) -> str:
    meta, st = pod.get("metadata", {}), pod.get("status", {})
    lines = [
        f"pod {meta.get('name')} phase={st.get('phase')} node={pod.get('spec', {}).get('nodeName')}"
    ]
    for c in st.get("conditions", []):
        if c.get("status") != "True":
            lines.append(f"  condition {c.get('type')}=False reason={c.get('reason', '')}")
    for cs in st.get("containerStatuses", []) + st.get("initContainerStatuses", []):
        state = cs.get("state", {})
        kind = next(iter(state), "unknown")
        detail = state.get(kind, {})
        last = cs.get("lastState", {}).get("terminated", {})
        lines.append(
            f"  container {cs.get('name')} ready={cs.get('ready')}"
            f" restarts={cs.get('restartCount')} state={kind}"
            f" reason={detail.get('reason', '')} image={cs.get('image')}"
        )
        if last:
            lines.append(
                f"    last terminated: reason={last.get('reason')} exitCode={last.get('exitCode')}"
            )
    return "\n".join(lines)


def summarize_events(events: list[dict], involved: str | None = None) -> str:
    rows = []
    for e in events:
        obj = e.get("involvedObject") or e.get("regarding") or {}
        if involved and involved not in obj.get("name", ""):
            continue
        ts = (
            e.get("lastTimestamp")
            or e.get("eventTime")
            or e.get("metadata", {}).get("creationTimestamp")
            or ""
        )
        rows.append(
            (
                ts,
                f"{ts} {e.get('type')} {e.get('reason')} {obj.get('kind')}/{obj.get('name')}: "
                f"{(e.get('message') or e.get('note') or '').strip()} (x{e.get('count', 1)})",
            )
        )
    rows.sort()
    return "\n".join(r for _, r in rows[-MAX_EVENTS:]) or "no matching events"


def summarize_rollouts(replicasets: list[dict], deployment: str, limit: int = 3) -> str:
    owned = [
        rs
        for rs in replicasets
        if any(
            o.get("name") == deployment for o in rs.get("metadata", {}).get("ownerReferences", [])
        )
    ]
    owned.sort(
        key=lambda rs: int(
            rs["metadata"].get("annotations", {}).get("deployment.kubernetes.io/revision", "0")
        ),
        reverse=True,
    )
    lines = []
    for rs in owned[:limit]:
        m, st = rs["metadata"], rs.get("status", {})
        rev = m.get("annotations", {}).get("deployment.kubernetes.io/revision", "?")
        images = ",".join(
            c.get("image", "")
            for c in rs.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
        )
        lines.append(
            f"revision {rev} created={m.get('creationTimestamp')} "
            f"ready={st.get('readyReplicas', 0)}/{st.get('replicas', 0)} images={images}"
        )
    return "\n".join(lines) or f"no ReplicaSets found for deployment {deployment}"


def _deployment_of(alert: Alert, pod: dict | None) -> str | None:
    if alert.labels.get("deployment"):
        return alert.labels["deployment"]
    for o in (pod or {}).get("metadata", {}).get("ownerReferences", []):
        if o.get("kind") == "ReplicaSet":
            return o["name"].rsplit("-", 1)[0]
    return None


class ContextGatherer:
    def __init__(self, kube: KubeClient, loki: LokiClient | None = None):
        self.kube, self.loki = kube, loki

    async def gather(self, alert: Alert) -> IncidentContext:
        ctx = IncidentContext(alert)
        ns, pod_name = alert.namespace, alert.labels.get("pod")
        pod: dict | None = None

        async def section(name: str, coro):
            try:
                ctx.sections[name] = _clip(await coro)
            except Exception as exc:  # report, never abort
                ctx.errors[name] = f"{type(exc).__name__}: {exc}"[:300]

        if ns and pod_name:
            try:
                pod = await self.kube.get(f"/api/v1/namespaces/{ns}/pods/{pod_name}")
                ctx.sections["pod status"] = _clip(summarize_pod(pod))
            except Exception as exc:
                ctx.errors["pod status"] = f"{type(exc).__name__}: {exc}"[:300]

        tasks = []
        if ns:
            tasks.append(
                section("events", self._events(ns, pod_name or alert.labels.get("deployment")))
            )
            dep = _deployment_of(alert, pod)
            if dep:
                tasks.append(section("recent rollouts", self._rollouts(ns, dep)))
            if self.loki is not None:
                tasks.append(section("logs (last 15m)", self._logs(ns, pod_name, dep)))
        elif alert.labels.get("node"):
            tasks.append(section("node", self._node(alert.labels["node"])))
        await asyncio.gather(*tasks)
        return ctx

    async def _events(self, ns: str, involved: str | None) -> str:
        data = await self.kube.get(f"/api/v1/namespaces/{ns}/events", {"limit": 200})
        return summarize_events(data.get("items", []), involved)

    async def _rollouts(self, ns: str, deployment: str) -> str:
        data = await self.kube.get(f"/apis/apps/v1/namespaces/{ns}/replicasets")
        return summarize_rollouts(data.get("items", []), deployment)

    async def _logs(self, ns: str, pod: str | None, dep: str | None) -> str:
        assert self.loki is not None
        if pod:
            selector = f'{{namespace="{ns}", pod="{pod}"}}'
        elif dep:
            selector = f'{{namespace="{ns}", pod=~"{dep}-.*"}}'
        else:
            selector = f'{{namespace="{ns}"}} |~ "(?i)(error|panic|fatal|exception)"'
        lines = await self.loki.query_range(selector)
        return "\n".join(lines) or "no log lines"

    async def _node(self, node: str) -> str:
        n = await self.kube.get(f"/api/v1/nodes/{node}")
        conds = n.get("status", {}).get("conditions", [])
        return "\n".join(
            f"{c.get('type')}={c.get('status')} {c.get('reason', '')}: {c.get('message', '')}"
            for c in conds
        )
