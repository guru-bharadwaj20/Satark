"""Scan report and its compact termination-message encoding."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field

# The kubelet truncates termination messages at 4096 bytes.
MAX_MESSAGE_BYTES = 4096
MAX_URL_CHARS = 512


@dataclass
class Forms:
    password: bool = False
    otp: bool = False
    card_number: bool = False


@dataclass
class ScanReport:
    final_url: str = ""
    title: str = ""
    redirect_chain: list[str] = field(default_factory=list)
    forms: Forms = field(default_factory=Forms)
    downloads: list[str] = field(default_factory=list)
    screenshot_key: str = ""
    error: str = ""


def _clip(value: str, n: int = MAX_URL_CHARS) -> str:
    return value if len(value) <= n else value[: n - 3] + "..."


def encode(report: ScanReport, limit: int = MAX_MESSAGE_BYTES) -> str:
    """Serialise the report as compact JSON that fits in ``limit`` bytes.

    Long fields are clipped and the redirect chain is shortened from the
    middle (keeping the first and last hops) until the document fits.
    """
    data = asdict(report)
    data["final_url"] = _clip(data["final_url"])
    data["title"] = _clip(data["title"], 200)
    data["error"] = _clip(data["error"], 500)
    data["redirect_chain"] = [_clip(u) for u in data["redirect_chain"]]
    data["downloads"] = [_clip(d, 200) for d in data["downloads"][:10]]

    def dump() -> str:
        return json.dumps(data, separators=(",", ":"), ensure_ascii=False)

    out = dump()
    while len(out.encode()) > limit and len(data["redirect_chain"]) > 2:
        del data["redirect_chain"][len(data["redirect_chain"]) // 2]
        out = dump()
    if len(out.encode()) > limit:
        data["redirect_chain"] = [_clip(u, 128) for u in data["redirect_chain"]]
        data["downloads"] = data["downloads"][:3]
        out = dump()
    return out
