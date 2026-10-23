"""Generate Satark's Grafana dashboards as code.

Dashboards are defined in Python below and rendered to
gitops/platform/monitoring/dashboards/*.json, which Argo CD ships to Grafana
as labelled ConfigMaps. Never edit the JSON by hand: change this file and run

    python3 scripts/dashboards/generate.py          # rewrite all dashboards
    python3 scripts/dashboards/generate.py --check  # CI: fail on drift
"""
import argparse
import json
import sys
from pathlib import Path

OUT_DIR = Path(__file__).resolve().parents[2] / "gitops" / "platform" / "monitoring" / "dashboards"

DS = {"type": "prometheus", "uid": "${datasource}"}
LOKI = {"type": "loki", "uid": "loki"}


class Dash:
    def __init__(self, uid, title, tags, desc, extra_vars=()):
        self.uid, self.title, self.tags, self.desc = uid, title, tags, desc
        self.panels, self.x, self.y, self.rowh, self.id = [], 0, 0, 0, 1
        self.vars = [{
            "name": "datasource", "type": "datasource", "query": "prometheus",
            "current": {"text": "Prometheus", "value": "prometheus"}, "hide": 0, "label": "Data source",
        }, *extra_vars]

    def row(self, title):
        self.newline()
        self.panels.append({"id": self.nid(), "type": "row", "title": title, "collapsed": False,
                            "gridPos": {"h": 1, "w": 24, "x": 0, "y": self.y}, "panels": []})
        self.y += 1

    def newline(self):
        if self.x:
            self.y += self.rowh
            self.x, self.rowh = 0, 0

    def nid(self):
        self.id += 1
        return self.id - 1

    def add(self, ptype, title, targets, w=12, h=8, unit="short", desc="", ds=DS, **opts):
        if self.x + w > 24:
            self.newline()
        p = {
            "id": self.nid(), "type": ptype, "title": title, "description": desc, "datasource": ds,
            "gridPos": {"h": h, "w": w, "x": self.x, "y": self.y},
            "targets": [
                {"refId": chr(65 + i), "expr": e, "legendFormat": lf, "datasource": ds}
                for i, (e, lf) in enumerate(targets)
            ],
            "fieldConfig": {"defaults": {"unit": unit, **opts.pop("defaults", {})}, "overrides": []},
            "options": opts.pop("options", {}),
        }
        if ptype == "timeseries":
            p["fieldConfig"]["defaults"].setdefault("custom", {"lineWidth": 1, "fillOpacity": 10})
            p["options"].setdefault("legend", {"displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"]})
        if ptype == "stat":
            p["options"].setdefault("reduceOptions", {"calcs": ["lastNotNull"]})
        p.update(opts)
        self.panels.append(p)
        self.x += w
        self.rowh = max(self.rowh, h)

    def stat(self, title, expr, unit="short", w=6, thresholds=None, desc=""):
        d = {}
        if thresholds:
            d["thresholds"] = {"mode": "absolute", "steps": thresholds}
            d["color"] = {"mode": "thresholds"}
        self.add("stat", title, [(expr, "")], w=w, h=4, unit=unit, desc=desc, defaults=d)

    def ts(self, title, targets, unit="short", w=12, desc="", stack=False):
        d = {"custom": {"lineWidth": 1, "fillOpacity": 30 if stack else 10,
                        "stacking": {"mode": "normal" if stack else "none"}}}
        self.add("timeseries", title, targets, w=w, unit=unit, desc=desc, defaults=d)

    def json(self):
        return {
            "uid": self.uid, "title": self.title, "tags": ["satark", *self.tags], "description": self.desc,
            "editable": False, "schemaVersion": 39, "version": 1, "timezone": "browser",
            "refresh": "1m", "time": {"from": "now-24h", "to": "now"}, "graphTooltip": 1,
            "templating": {"list": self.vars}, "annotations": {"list": []}, "links": [],
            "panels": self.panels,
        }


def qvar(name, query, label, multi=True, include_all=True):
    return {"name": name, "label": label, "type": "query", "datasource": DS,
            "query": {"query": query, "refId": name}, "definition": query, "refresh": 2,
            "multi": multi, "includeAll": include_all, "allValue": ".*",
            "current": {"text": "All", "value": "$__all"}, "sort": 1}


def cluster_health():
    d = Dash("satark-cluster-health", "Satark / Cluster health", ["cluster"],
             "Kubernetes control plane, workloads and capacity.")
    d.row("Overview")
    d.stat("Nodes Ready", 'sum(kube_node_status_condition{condition="Ready",status="true"})')
    d.stat("Nodes NotReady", 'sum(kube_node_status_condition{condition="Ready",status=~"false|unknown"})',
           thresholds=[{"color": "green", "value": None}, {"color": "red", "value": 1}])
    d.stat("Pods not running", 'sum(kube_pod_status_phase{phase=~"Pending|Failed|Unknown"})',
           thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 1}, {"color": "red", "value": 5}])
    d.stat("Firing alerts", 'count(ALERTS{alertstate="firing",severity=~"critical|warning"}) or vector(0)',
           thresholds=[{"color": "green", "value": None}, {"color": "red", "value": 1}])
    d.stat("API server availability (1h)",
           '1 - (sum(rate(apiserver_request_total{code=~"5.."}[1h])) / sum(rate(apiserver_request_total[1h])))',
           unit="percentunit", thresholds=[{"color": "red", "value": None}, {"color": "green", "value": 0.99}])
    d.stat("etcd has leader", 'min(etcd_server_has_leader)',
           thresholds=[{"color": "red", "value": None}, {"color": "green", "value": 1}])
    d.stat("Cluster CPU requested", 'sum(kube_pod_container_resource_requests{resource="cpu"}) / sum(kube_node_status_allocatable{resource="cpu"})',
           unit="percentunit", thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 0.8}, {"color": "red", "value": 0.95}])
    d.stat("Cluster memory requested", 'sum(kube_pod_container_resource_requests{resource="memory"}) / sum(kube_node_status_allocatable{resource="memory"})',
           unit="percentunit", thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 0.8}, {"color": "red", "value": 0.95}])
    d.row("Workloads")
    d.ts("Container restarts (15m) by namespace",
         [('sum by (namespace) (increase(kube_pod_container_status_restarts_total[15m]))', "{{namespace}}")])
    d.ts("Waiting containers by reason",
         [('sum by (reason) (kube_pod_container_status_waiting_reason)', "{{reason}}")])
    d.ts("Deployments with unavailable replicas",
         [('sum by (namespace, deployment) (kube_deployment_status_replicas_unavailable) > 0', "{{namespace}}/{{deployment}}")])
    d.ts("PVC usage", [('max by (namespace, persistentvolumeclaim) (kubelet_volume_stats_used_bytes / kubelet_volume_stats_capacity_bytes)',
                        "{{namespace}}/{{persistentvolumeclaim}}")], unit="percentunit")
    d.row("Control plane")
    d.ts("API server request rate by code", [('sum by (code) (rate(apiserver_request_total[5m]))', "{{code}}")], unit="reqps")
    d.ts("API server p99 latency (non-watch)",
         [('histogram_quantile(0.99, sum by (le, verb) (rate(apiserver_request_duration_seconds_bucket{verb!~"WATCH|CONNECT"}[5m])))', "{{verb}}")], unit="s")
    d.ts("etcd WAL fsync p99", [('histogram_quantile(0.99, sum by (le, instance) (rate(etcd_disk_wal_fsync_duration_seconds_bucket[5m])))', "{{instance}}")], unit="s")
    d.ts("CoreDNS responses by rcode", [('sum by (rcode) (rate(coredns_dns_responses_total[5m]))', "{{rcode}}")], unit="reqps")
    d.ts("Cilium drops by reason", [('sum by (reason) (rate(cilium_drop_count_total[5m]))', "{{reason}}")], unit="pps")
    d.ts("CPU usage by namespace", [('sum by (namespace) (rate(container_cpu_usage_seconds_total{container!=""}[5m]))', "{{namespace}}")], unit="short")
    return d


def node_linux():
    node = qvar("node", 'label_values(node_uname_info, nodename)', "Node")
    d = Dash("satark-node-linux", "Satark / Node and Linux", ["cluster", "node"],
             "Per-node Linux metrics from node-exporter.", [node])
    sel = 'instance=~"$instance"'
    # Hidden variable: maps the chosen node names to node-exporter instances.
    d.vars.append(qvar("instance", 'label_values(node_uname_info{nodename=~"$node"}, instance)', "Instance"))
    d.vars[-1]["hide"] = 2
    d.row("Saturation")
    d.stat("Uptime", f'min(time() - node_boot_time_seconds{{{sel}}})', unit="s")
    d.stat("CPU busy", f'1 - avg(rate(node_cpu_seconds_total{{mode="idle",{sel}}}[5m]))', unit="percentunit",
           thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 0.75}, {"color": "red", "value": 0.9}])
    d.stat("Memory used", f'1 - sum(node_memory_MemAvailable_bytes{{{sel}}}) / sum(node_memory_MemTotal_bytes{{{sel}}})', unit="percentunit",
           thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 0.8}, {"color": "red", "value": 0.9}])
    d.stat("Root FS used", f'1 - min(node_filesystem_avail_bytes{{mountpoint="/",fstype!="rootfs",{sel}}} / node_filesystem_size_bytes{{mountpoint="/",fstype!="rootfs",{sel}}})',
           unit="percentunit", thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 0.8}, {"color": "red", "value": 0.9}])
    d.ts("CPU by mode", [(f'sum by (instance, mode) (rate(node_cpu_seconds_total{{mode!="idle",{sel}}}[5m])) / scalar(count(node_cpu_seconds_total{{mode="idle",{sel}}}))', "{{instance}} {{mode}}")], unit="percentunit", stack=True)
    d.ts("Load average (1m) per core", [(f'node_load1{{{sel}}} / on(instance) count by (instance) (node_cpu_seconds_total{{mode="idle",{sel}}})', "{{instance}}")])
    d.ts("Memory", [(f'node_memory_MemTotal_bytes{{{sel}}} - node_memory_MemAvailable_bytes{{{sel}}}', "used {{instance}}"),
                    (f'node_memory_MemAvailable_bytes{{{sel}}}', "available {{instance}}")], unit="bytes")
    d.ts("PSI pressure (some, 1m)", [(f'rate(node_pressure_cpu_waiting_seconds_total{{{sel}}}[5m])', "cpu {{instance}}"),
                                     (f'rate(node_pressure_memory_waiting_seconds_total{{{sel}}}[5m])', "memory {{instance}}"),
                                     (f'rate(node_pressure_io_waiting_seconds_total{{{sel}}}[5m])', "io {{instance}}")], unit="percentunit")
    d.row("Disk and network")
    d.ts("Disk I/O utilisation", [(f'rate(node_disk_io_time_seconds_total{{device!~"dm-.*|loop.*",{sel}}}[5m])', "{{instance}} {{device}}")], unit="percentunit")
    d.ts("Filesystem free", [(f'node_filesystem_avail_bytes{{fstype!~"tmpfs|overlay|squashfs",{sel}}}', "{{instance}} {{mountpoint}}")], unit="bytes")
    d.ts("Network throughput", [(f'rate(node_network_receive_bytes_total{{device!~"lo|veth.*|lxc.*|cilium.*",{sel}}}[5m])', "rx {{instance}} {{device}}"),
                                (f'-rate(node_network_transmit_bytes_total{{device!~"lo|veth.*|lxc.*|cilium.*",{sel}}}[5m])', "tx {{instance}} {{device}}")], unit="Bps")
    d.ts("Network errors and drops", [(f'rate(node_network_receive_errs_total{{{sel}}}[5m]) + rate(node_network_receive_drop_total{{{sel}}}[5m])', "rx {{instance}} {{device}}"),
                                      (f'rate(node_network_transmit_errs_total{{{sel}}}[5m]) + rate(node_network_transmit_drop_total{{{sel}}}[5m])', "tx {{instance}} {{device}}")], unit="pps")
    d.ts("Conntrack usage", [(f'node_nf_conntrack_entries{{{sel}}} / node_nf_conntrack_entries_limit{{{sel}}}', "{{instance}}")], unit="percentunit")
    d.ts("Clock offset (timex)", [(f'node_timex_offset_seconds{{{sel}}}', "{{instance}}")], unit="s")
    return d


def product_usage():
    ch = qvar("channel", 'label_values(satark_scans_total, channel)', "Channel")
    d = Dash("satark-product-usage", "Satark / Product usage", ["product"],
             "How people use Satark: scans by channel, kind, verdict and scam category. Counts only; no user data.", [ch])
    f = 'channel=~"$channel"'
    d.row("Today")
    d.stat("Scans (24h)", f'sum(increase(satark_scans_total{{{f}}}[24h]))')
    d.stat("High-risk verdicts (24h)", f'sum(increase(satark_scans_total{{verdict="high_risk",{f}}}[24h]))')
    d.stat("High-risk share (24h)", f'sum(increase(satark_scans_total{{verdict="high_risk",{f}}}[24h])) / sum(increase(satark_scans_total{{{f}}}[24h]))', unit="percentunit")
    d.stat("Scans per minute (now)", f'sum(rate(satark_scans_total{{{f}}}[5m])) * 60')
    d.row("Volume")
    d.ts("Scans by channel", [(f'sum by (channel) (rate(satark_scans_total{{{f}}}[$__rate_interval])) * 60', "{{channel}}")], unit="short", stack=True, desc="Scans per minute.")
    d.ts("Scans by kind", [(f'sum by (kind) (rate(satark_scans_total{{{f}}}[$__rate_interval])) * 60', "{{kind}}")], unit="short", stack=True, desc="Scans per minute.")
    d.ts("Verdicts", [(f'sum by (verdict) (rate(satark_scans_total{{{f}}}[$__rate_interval])) * 60', "{{verdict}}")], stack=True)
    d.add("bargauge", "Top scam categories (24h)",
          [(f'topk(10, sum by (category) (increase(satark_scans_total{{category!="none",{f}}}[24h])))', "{{category}}")],
          options={"orientation": "horizontal", "displayMode": "gradient", "reduceOptions": {"calcs": ["lastNotNull"]}})
    d.row("Trends")
    d.ts("Daily scans (7d)", [(f'sum(increase(satark_scans_total{{{f}}}[1d]))', "scans/day")], w=12)
    d.add("piechart", "Verdict mix (7d)", [(f'sum by (verdict) (increase(satark_scans_total{{{f}}}[7d]))', "{{verdict}}")],
          options={"reduceOptions": {"calcs": ["lastNotNull"]}, "pieType": "donut", "legend": {"displayMode": "table", "placement": "right", "values": ["percent"]}})
    return d


def scan_pipeline():
    svc = qvar("service", 'label_values(satark_http_requests_total, service)', "Service")
    d = Dash("satark-scan-pipeline", "Satark / Scan pipeline", ["pipeline", "red"],
             "RED metrics per service, LLM and cache behaviour, and error logs.", [svc])
    s = 'service=~"$service"'
    d.row("RED per service")
    d.ts("Request rate", [(f'sum by (service) (rate(satark_http_requests_total{{{s}}}[$__rate_interval]))', "{{service}}")], unit="reqps", w=8)
    d.ts("Error ratio (5xx)", [(f'sum by (service) (rate(satark_http_requests_total{{code=~"5..",{s}}}[$__rate_interval])) / sum by (service) (rate(satark_http_requests_total{{{s}}}[$__rate_interval]))', "{{service}}")], unit="percentunit", w=8)
    d.ts("p95 latency", [(f'histogram_quantile(0.95, sum by (service, le) (rate(satark_http_request_duration_seconds_bucket{{{s}}}[$__rate_interval])))', "{{service}}")], unit="s", w=8)
    d.ts("Latency percentiles by route", [
        (f'histogram_quantile(0.50, sum by (route, le) (rate(satark_http_request_duration_seconds_bucket{{{s}}}[$__rate_interval])))', "p50 {{route}}"),
        (f'histogram_quantile(0.99, sum by (route, le) (rate(satark_http_request_duration_seconds_bucket{{{s}}}[$__rate_interval])))', "p99 {{route}}")], unit="s")
    d.ts("Requests by status code", [(f'sum by (code) (rate(satark_http_requests_total{{{s}}}[$__rate_interval]))', "{{code}}")], unit="reqps", stack=True)
    d.ts("In-flight requests", [(f'sum by (service) (satark_http_requests_in_flight{{{s}}})', "{{service}}")])
    d.ts("Pod restarts (1h)", [('sum by (pod) (increase(kube_pod_container_status_restarts_total{namespace="satark"}[1h]))', "{{pod}}")])
    d.row("LLM")
    d.ts("LLM calls by outcome", [(f'sum by (provider, outcome) (rate(satark_llm_calls_total{{{s}}}[$__rate_interval])) * 60', "{{provider}} {{outcome}}")], w=8, stack=True, desc="Calls per minute.")
    d.ts("LLM p95 latency", [(f'histogram_quantile(0.95, sum by (provider, le) (rate(satark_llm_call_duration_seconds_bucket{{{s}}}[$__rate_interval])))', "{{provider}}")], unit="s", w=8)
    d.ts("LLM tokens per hour", [(f'sum by (provider, direction) (rate(satark_llm_tokens_total{{{s}}}[$__rate_interval])) * 3600', "{{provider}} {{direction}}")], w=8)
    d.row("Caches")
    d.ts("Cache hit ratio", [(f'sum by (cache) (rate(satark_cache_requests_total{{result="hit",{s}}}[$__rate_interval])) / sum by (cache) (rate(satark_cache_requests_total{{{s}}}[$__rate_interval]))', "{{cache}}")], unit="percentunit")
    d.ts("Cache lookups", [(f'sum by (cache, result) (rate(satark_cache_requests_total{{{s}}}[$__rate_interval]))', "{{cache}} {{result}}")], unit="ops")
    d.row("Logs")
    d.add("logs", "Error logs", [('{namespace="satark", service=~"$service", level="ERROR"}', "")], w=24, h=10, ds=LOKI,
          options={"showTime": True, "wrapLogMessage": True, "sortOrder": "Descending"})
    return d


def slo():
    d = Dash("satark-slo", "Satark / SLOs and error budget", ["slo"],
             "Gateway API availability (99.5%) and latency (95% under 800 ms) over 30 days. "
             "Built on slo:* recording rules.")
    for name, label in (("availability", "Availability"), ("latency", "Latency")):
        f = f'slo="{name}",service="gateway-api"'
        d.row(f"{label} SLO")
        d.stat(f"{label} objective", f'max(slo:objective:ratio{{{f}}})', unit="percentunit", w=4)
        d.stat(f"{label} SLI (30d)", f'1 - max(slo:sli_error:ratio_rate30d{{{f}}})', unit="percentunit", w=5,
               thresholds=[{"color": "red", "value": None}, {"color": "green", "value": 0.995 if name == "availability" else 0.95}])
        d.stat("Error budget remaining", f'max(slo:error_budget_remaining:ratio{{{f}}})', unit="percentunit", w=5,
               thresholds=[{"color": "red", "value": None}, {"color": "orange", "value": 0.25}, {"color": "green", "value": 0.5}])
        d.stat("Burn rate (1h)", f'max(slo:sli_error:ratio_rate1h{{{f}}}) / (1 - max(slo:objective:ratio{{{f}}}))', w=5,
               thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 6}, {"color": "red", "value": 14.4}])
        d.stat("Burn rate (6h)", f'max(slo:sli_error:ratio_rate6h{{{f}}}) / (1 - max(slo:objective:ratio{{{f}}}))', w=5,
               thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 1}, {"color": "red", "value": 6}])
        d.ts("Burn rate by window", [(f'max(slo:sli_error:ratio_rate{w}{{{f}}}) / (1 - max(slo:objective:ratio{{{f}}}))', w)
                                     for w in ("5m", "30m", "1h", "6h", "1d", "3d")],
             desc="1 = spending budget exactly at the sustainable rate. Page at 14.4 (1h) and 6 (6h).")
        d.ts("Error budget remaining (30d)", [(f'max(slo:error_budget_remaining:ratio{{{f}}})', "remaining")], unit="percentunit")
    d.row("Alerts")
    d.add("table", "Firing SLO alerts", [('ALERTS{alertstate="firing", alertname=~"Satark.*Burn.*"}', "")], w=24, h=6,
          options={"showHeader": True})
    return d


BUILDERS = {"cluster-health": cluster_health, "node-linux": node_linux, "product-usage": product_usage,
            "scan-pipeline": scan_pipeline, "slo": slo}

def main(argv):
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--out", type=Path, default=OUT_DIR)
    ap.add_argument("--check", action="store_true", help="fail if committed JSON differs from the generator")
    ap.add_argument("names", nargs="*", default=sorted(BUILDERS))
    args = ap.parse_args(argv)
    stale = []
    for name in args.names:
        rendered = json.dumps(BUILDERS[name]().json(), indent=2) + "\n"
        path = args.out / f"{name}.json"
        if args.check:
            if not path.exists() or path.read_text() != rendered:
                stale.append(str(path))
        else:
            path.write_text(rendered)
    if stale:
        print("Dashboards out of date; run scripts/dashboards/generate.py:\n  " + "\n  ".join(stale))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
