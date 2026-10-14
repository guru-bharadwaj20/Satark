from pathlib import Path

import yaml

DOCS = list(yaml.safe_load_all((Path(__file__).parents[1] / "deploy" / "rbac.yaml").read_text()))
READ_VERBS = {"get", "list", "watch"}


def _role():
    return next(d for d in DOCS if d["kind"] == "ClusterRole")


def test_only_read_verbs():
    for rule in _role()["rules"]:
        assert set(rule["verbs"]) <= READ_VERBS, rule


def test_no_wildcards():
    for rule in _role()["rules"]:
        for field in ("apiGroups", "resources", "verbs"):
            assert "*" not in rule[field], rule


def test_no_secret_or_exec_access():
    forbidden = {
        "secrets",
        "configmaps",
        "pods/exec",
        "pods/attach",
        "pods/portforward",
        "serviceaccounts/token",
    }
    for rule in _role()["rules"]:
        assert not forbidden & set(rule["resources"]), rule


def test_binding_targets_opsbot_sa():
    b = next(d for d in DOCS if d["kind"] == "ClusterRoleBinding")
    assert b["roleRef"]["name"] == _role()["metadata"]["name"]
    assert b["subjects"] == [{"kind": "ServiceAccount", "name": "opsbot", "namespace": "satark"}]
