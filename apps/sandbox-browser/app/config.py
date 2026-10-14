"""Runtime settings, read from the environment set by linkscan-operator."""

from __future__ import annotations

import os
from dataclasses import dataclass
from urllib.parse import urlparse

MAX_TIMEOUT_SECONDS = 300


class ConfigError(ValueError):
    """Raised when the environment does not describe a valid scan."""


@dataclass(frozen=True)
class Settings:
    target_url: str
    scan_id: str
    timeout_seconds: int = 60
    result_path: str = "/dev/termination-log"
    viewport_width: int = 1280
    viewport_height: int = 800
    user_agent: str = (
        "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36"
    )

    @property
    def navigation_budget_ms(self) -> int:
        """Leave headroom under the pod deadline to write the result."""
        return max(5, self.timeout_seconds - 8) * 1000


def _int(env: dict[str, str], key: str, default: int) -> int:
    raw = env.get(key, "")
    if not raw:
        return default
    try:
        return int(raw)
    except ValueError as exc:
        raise ConfigError(f"{key} must be an integer") from exc


def load(env: dict[str, str] | None = None) -> Settings:
    env = dict(os.environ if env is None else env)
    url = env.get("TARGET_URL", "").strip()
    parsed = urlparse(url)
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        raise ConfigError("TARGET_URL must be an absolute http(s) URL")
    timeout = _int(env, "TIMEOUT_SECONDS", 60)
    if not 10 <= timeout <= MAX_TIMEOUT_SECONDS:
        raise ConfigError(f"TIMEOUT_SECONDS must be between 10 and {MAX_TIMEOUT_SECONDS}")
    return Settings(
        target_url=url,
        scan_id=env.get("SCAN_ID", "local"),
        timeout_seconds=timeout,
        result_path=env.get("RESULT_PATH", "/dev/termination-log"),
    )
