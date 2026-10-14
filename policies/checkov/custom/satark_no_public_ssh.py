"""CKV_SATARK_2: no security group rule may open SSH (22/tcp) to the internet.

Covers all three Terraform styles: inline ``ingress`` blocks on
``aws_security_group``, ``aws_security_group_rule``, and
``aws_vpc_security_group_ingress_rule``. Shell access to the edge goes through
SSM Session Manager, so port 22 is never needed.
"""

from __future__ import annotations

from typing import Any

from checkov.common.models.enums import CheckCategories, CheckResult
from checkov.terraform.checks.resource.base_resource_check import BaseResourceCheck

OPEN_CIDRS = {"0.0.0.0/0", "::/0"}
SSH_PORT = 22


def _first(value: Any) -> Any:
    """Checkov wraps HCL attribute values in single-item lists."""
    if isinstance(value, list):
        return value[0] if len(value) == 1 else value
    return value


def _as_list(value: Any) -> list[Any]:
    value = _first(value)
    if value is None:
        return []
    if isinstance(value, list):
        return value
    return [value]


def _to_int(value: Any, default: int) -> int:
    try:
        return int(_first(value))
    except (TypeError, ValueError):
        return default


def rule_opens_public_ssh(rule: dict[str, Any]) -> bool:
    protocol = str(_first(rule.get("protocol", rule.get("ip_protocol", "tcp")))).lower()
    if protocol not in {"tcp", "6", "-1", "all"}:
        return False

    if protocol in {"-1", "all"}:
        covers_22 = True
    else:
        from_port = _to_int(rule.get("from_port"), 0)
        to_port = _to_int(rule.get("to_port"), 65535)
        covers_22 = from_port <= SSH_PORT <= to_port
    if not covers_22:
        return False

    cidrs: list[Any] = []
    for key in ("cidr_blocks", "ipv6_cidr_blocks", "cidr_ipv4", "cidr_ipv6"):
        cidrs.extend(_as_list(rule.get(key)))
    return any(str(c) in OPEN_CIDRS for c in cidrs)


class NoPublicSSH(BaseResourceCheck):
    def __init__(self) -> None:
        super().__init__(
            name="Ensure no security group rule opens SSH (22/tcp) to 0.0.0.0/0 or ::/0",
            id="CKV_SATARK_2",
            categories=[CheckCategories.NETWORKING],
            supported_resources=[
                "aws_security_group",
                "aws_security_group_rule",
                "aws_vpc_security_group_ingress_rule",
            ],
        )

    def scan_resource_conf(self, conf: dict[str, Any]) -> CheckResult:
        if self.entity_type == "aws_security_group":
            rules = [r for r in conf.get("ingress", []) if isinstance(r, dict)]
        elif self.entity_type == "aws_security_group_rule":
            if str(_first(conf.get("type"))) != "ingress":
                return CheckResult.PASSED
            rules = [conf]
        else:
            rules = [conf]
        if any(rule_opens_public_ssh(r) for r in rules):
            return CheckResult.FAILED
        return CheckResult.PASSED


check = NoPublicSSH()
