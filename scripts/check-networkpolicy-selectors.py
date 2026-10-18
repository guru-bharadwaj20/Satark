#!/usr/bin/env python3
"""Fail if a NetworkPolicy selects nothing, allows traffic from nothing, or
leaves pods without DNS egress (GD-04 finding).

Reads rendered manifests (multi-document YAML) on stdin, for example every
chart rendered for one environment:

    for c in charts/*/; do helm template "$(basename "$c")" "$c" -f ...; done \
      | scripts/check-networkpolicy-selectors.py

Checks, for each NetworkPolicy:
  - spec.podSelector matches the pod template of at least one workload
  - every ingress `from` / egress `to` podSelector without a namespaceSelector
    matches at least one workload (a misspelt label matches nothing and
    silently drops the traffic)
  - every workload selected by a policy with Egress has some egress rule,
    across all policies selecting it, that allows port 53 (policies add up,
    so one rule anywhere is enough)
Only matchLabels are checked; matchExpressions are skipped. A missing
namespace is treated as "same namespace", since `helm template` often omits it.
Requires PyYAML.
"""
import sys

import yaml

WORKLOADS = {"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job"}


def pod_labels(doc):
    spec = doc.get("spec") or {}
    if doc["kind"] == "CronJob":
        spec = ((spec.get("jobTemplate") or {}).get("spec")) or {}
    return ((spec.get("template") or {}).get("metadata") or {}).get("labels") or {}


def same_ns(a, b):
    return not a or not b or a == b


def matches(selector, labels):
    want = (selector or {}).get("matchLabels") or {}
    return all(labels.get(k) == v for k, v in want.items())


def allows_dns(rule):
    ports = rule.get("ports")
    if not ports:
        return True
    return any(str(p.get("port", "")) in ("53", "dns", "dns-tcp", "") for p in ports)


def main():
    docs = [d for d in yaml.safe_load_all(sys.stdin) if isinstance(d, dict) and d.get("kind")]
    pods = []  # (namespace, name, labels)
    for d in docs:
        if d["kind"] in WORKLOADS | {"CronJob"}:
            md = d.get("metadata") or {}
            pods.append((md.get("namespace"), f'{d["kind"]}/{md.get("name")}', pod_labels(d)))
    policies = [d for d in docs if d["kind"] == "NetworkPolicy"]

    errors = []
    selected_by = {}  # pod name -> [policy]
    for np in policies:
        md = np.get("metadata") or {}
        ns, name = md.get("namespace"), md.get("name")
        spec = np.get("spec") or {}
        sel = spec.get("podSelector") or {}
        hits = [p for p in pods if same_ns(ns, p[0]) and matches(sel, p[2])]
        if sel.get("matchLabels") and not hits:
            errors.append(f"NetworkPolicy {name}: podSelector {sel['matchLabels']} matches no workload")
        for p in hits:
            selected_by.setdefault(p[1], []).append(np)
        for kind, key in (("ingress", "from"), ("egress", "to")):
            for rule in spec.get(kind) or []:
                for peer in rule.get(key) or []:
                    ps = peer.get("podSelector")
                    if ps is None or "namespaceSelector" in peer or not ps.get("matchLabels"):
                        continue
                    if not any(same_ns(ns, p[0]) and matches(ps, p[2]) for p in pods):
                        errors.append(f"NetworkPolicy {name}: {kind} {key} podSelector "
                                      f"{ps['matchLabels']} matches no workload")

    for pod, nps in sorted(selected_by.items()):
        if not any("Egress" in (np.get("spec") or {}).get("policyTypes", []) for np in nps):
            continue
        rules = [r for np in nps for r in (np.get("spec") or {}).get("egress") or []]
        if not any(allows_dns(r) for r in rules):
            names = ", ".join(np["metadata"]["name"] for np in nps)
            errors.append(f"{pod}: egress is restricted ({names}) but no rule allows DNS (port 53)")

    for e in errors:
        print(f"::error::{e}")
    print(f"checked {len(policies)} NetworkPolicies against {len(pods)} workloads: "
          f"{len(errors)} problem(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
