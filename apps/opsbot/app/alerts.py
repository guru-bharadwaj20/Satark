"""Alertmanager webhook payload (version 4)."""

from __future__ import annotations

from datetime import datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field


class Alert(BaseModel):
    model_config = ConfigDict(extra="ignore")

    status: Literal["firing", "resolved"]
    labels: dict[str, str] = Field(default_factory=dict)
    annotations: dict[str, str] = Field(default_factory=dict)
    startsAt: datetime | None = None
    endsAt: datetime | None = None
    generatorURL: str = ""
    fingerprint: str = ""

    @property
    def name(self) -> str:
        return self.labels.get("alertname", "unknown")

    @property
    def namespace(self) -> str:
        return self.labels.get("namespace", "")

    @property
    def severity(self) -> str:
        return self.labels.get("severity", "none")

    @property
    def key(self) -> str:
        """Stable identity used for cooldowns: fingerprint or alertname+labels."""
        if self.fingerprint:
            return self.fingerprint
        return self.name + "|" + ",".join(f"{k}={v}" for k, v in sorted(self.labels.items()))


class AlertmanagerPayload(BaseModel):
    model_config = ConfigDict(extra="ignore")

    version: str = "4"
    groupKey: str = ""
    status: Literal["firing", "resolved"]
    receiver: str = ""
    groupLabels: dict[str, str] = Field(default_factory=dict)
    commonLabels: dict[str, str] = Field(default_factory=dict)
    commonAnnotations: dict[str, str] = Field(default_factory=dict)
    externalURL: str = ""
    alerts: list[Alert] = Field(default_factory=list, max_length=200)
