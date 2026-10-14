import pytest

from app.llm.schema import Category, ClassificationResult
from app.llm.scoring import MAX_DOWN, MAX_UP, bounded_adjustment


def res(is_scam: bool, conf: float) -> ClassificationResult:
    return ClassificationResult(
        category=Category.OTHER_SCAM if is_scam else Category.LEGITIMATE,
        is_scam=is_scam,
        confidence=conf,
        explanation="x",
    )


def test_no_llm_keeps_rule_score():
    a = bounded_adjustment(42, None)
    assert a.final_score == 42 and not a.applied and a.reason == "llm_unavailable"


def test_full_confidence_scam_adds_max_up():
    assert bounded_adjustment(50, res(True, 1.0)).delta == MAX_UP


def test_full_confidence_legit_subtracts_max_down():
    assert bounded_adjustment(50, res(False, 1.0)).delta == -MAX_DOWN


def test_low_confidence_ignored():
    assert bounded_adjustment(50, res(True, 0.49)).delta == 0


def test_strong_rule_cannot_be_argued_down():
    a = bounded_adjustment(80, res(False, 1.0), strong_rule_hit=True)
    assert a.final_score == 80 and a.reason == "strong_rule_overrides_llm"


def test_clamped():
    assert bounded_adjustment(95, res(True, 1.0)).final_score == 100
    assert bounded_adjustment(3, res(False, 1.0)).final_score == 0


@pytest.mark.parametrize("score", range(0, 101, 5))
@pytest.mark.parametrize("conf", [0.0, 0.5, 0.6, 0.75, 0.9, 1.0])
@pytest.mark.parametrize("is_scam", [True, False])
def test_delta_always_bounded(score, conf, is_scam):
    a = bounded_adjustment(score, res(is_scam, conf))
    assert -MAX_DOWN <= a.delta <= MAX_UP
    assert 0 <= a.final_score <= 100
