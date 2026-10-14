"""Runtime configuration read from environment variables."""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Settings:
    port: int
    log_level: str
    rules_dir: Path
    max_text_chars: int


def load_settings() -> Settings:
    return Settings(
        port=int(os.getenv("PORT", "8000")),
        log_level=os.getenv("LOG_LEVEL", "info"),
        rules_dir=Path(os.getenv("RULES_DIR", "/app/rules")),
        max_text_chars=int(os.getenv("MAX_TEXT_CHARS", "5000")),
    )
