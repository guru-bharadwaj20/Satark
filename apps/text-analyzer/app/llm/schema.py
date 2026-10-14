"""Pydantic schema for the LLM's structured classification output."""

from __future__ import annotations

import json
import re
from enum import StrEnum
from typing import Literal

from pydantic import (
    BaseModel,
    ConfigDict,
    Field,
    ValidationError,
    field_validator,
    model_validator,
)

Language = Literal["en", "hi", "kn"]


class Category(StrEnum):
    KYC_BANKING = "kyc_banking"
    UTILITY_BILL = "utility_bill"
    OTP_PIN = "otp_pin"
    REMOTE_ACCESS = "remote_access"
    LOTTERY_PRIZE = "lottery_prize"
    TASK_JOB = "task_job"
    DELIVERY_PARCEL = "delivery_parcel"
    IMPERSONATION_AUTHORITY = "impersonation_authority"
    INVESTMENT = "investment"
    OTHER_SCAM = "other_scam"
    LEGITIMATE = "legitimate"


# Phrases user-facing text must never contain (CONTRIBUTING.md privacy and safety rules).
# Also rejects output that echoes the system prompt (a sign the model was hijacked).
_FORBIDDEN = re.compile(
    r"100\s*%\s*safe|completely safe|\bscammers?\b|\bfraudsters?\b"
    r"|message classifier|untrusted data|</?message>",
    re.I,
)


class SchemaError(ValueError):
    """The LLM output could not be parsed or failed validation."""


class ClassificationResult(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, use_enum_values=False)

    category: Category
    is_scam: bool
    confidence: float = Field(ge=0.0, le=1.0)
    reasons: list[str] = Field(default_factory=list, max_length=4)
    explanation: str = Field(min_length=1, max_length=600)
    language: Language = "en"

    @field_validator("reasons")
    @classmethod
    def _reasons(cls, v: list[str]) -> list[str]:
        out = [r.strip() for r in v if r and r.strip()]
        for r in out:
            if len(r) > 200:
                raise ValueError("reason too long")
        return out

    @model_validator(mode="after")
    def _consistent_and_safe(self) -> ClassificationResult:
        if self.category is Category.LEGITIMATE and self.is_scam:
            raise ValueError("category 'legitimate' contradicts is_scam=true")
        if self.category is not Category.LEGITIMATE and not self.is_scam:
            raise ValueError(f"category {self.category.value!r} requires is_scam=true")
        for text in [self.explanation, *self.reasons]:
            if _FORBIDDEN.search(text):
                raise ValueError("output contains forbidden wording")
        return self


_FENCE = re.compile(r"^```(?:json)?\s*|\s*```$", re.I)


def _extract_json(text: str) -> dict:
    s = _FENCE.sub("", text.strip())
    try:
        obj = json.loads(s)
    except json.JSONDecodeError:
        start, end = s.find("{"), s.rfind("}")
        if start < 0 or end <= start:
            raise SchemaError("no JSON object in LLM output") from None
        try:
            obj = json.loads(s[start : end + 1])
        except json.JSONDecodeError as exc:
            raise SchemaError("malformed JSON in LLM output") from exc
    if not isinstance(obj, dict):
        raise SchemaError("LLM output is not a JSON object")
    return obj


def parse_classification(text: str) -> ClassificationResult:
    """Parse and validate raw LLM text. Raises SchemaError on any problem."""
    data = _extract_json(text)
    try:
        return ClassificationResult.model_validate(data)
    except ValidationError as exc:
        raise SchemaError(f"LLM output failed validation: {exc.error_count()} error(s)") from exc


def json_schema() -> dict:
    return ClassificationResult.model_json_schema()
