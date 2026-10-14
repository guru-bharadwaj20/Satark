"""Bounded LLM influence on the rule-based risk score (see ADR-005).

The deterministic rule score stays the anchor. The LLM may nudge it, never
replace it:

* at most +MAX_UP points towards "scam" and -MAX_DOWN points towards "safe";
* the nudge scales with the LLM's confidence and is zero below MIN_CONFIDENCE;
* a strong rule hit (OTP/PIN request, remote-access app, ...) or a suspected
  prompt-injection attempt means the LLM can never argue the score down;
* the result is clamped to 0..100.
"""

from __future__ import annotations

from dataclasses import dataclass

from app.llm.schema import ClassificationResult

MAX_UP = 15
MAX_DOWN = 10
MIN_CONFIDENCE = 0.5


@dataclass(frozen=True, slots=True)
class ScoreAdjustment:
    rule_score: int
    delta: int
    final_score: int
    reason: str

    @property
    def applied(self) -> bool:
        return self.delta != 0


def _clamp(v: float, lo: int = 0, hi: int = 100) -> int:
    return int(max(lo, min(hi, round(v))))


def bounded_adjustment(
    rule_score: int,
    llm: ClassificationResult | None,
    *,
    strong_rule_hit: bool = False,
    injection_suspected: bool = False,
) -> ScoreAdjustment:
    base = _clamp(rule_score)
    if llm is None:
        return ScoreAdjustment(base, 0, base, "llm_unavailable")
    if llm.confidence < MIN_CONFIDENCE:
        return ScoreAdjustment(base, 0, base, "low_confidence")

    # Map confidence MIN_CONFIDENCE..1.0 linearly onto 0..1.
    weight = (llm.confidence - MIN_CONFIDENCE) / (1.0 - MIN_CONFIDENCE)
    if llm.is_scam:
        delta = round(MAX_UP * weight)
        reason = "llm_agrees_scam"
    else:
        if strong_rule_hit:
            return ScoreAdjustment(base, 0, base, "strong_rule_overrides_llm")
        if injection_suspected:
            return ScoreAdjustment(base, 0, base, "injection_suspected")
        delta = -round(MAX_DOWN * weight)
        reason = "llm_suggests_legitimate"

    final = _clamp(base + delta)
    return ScoreAdjustment(base, final - base, final, reason)
