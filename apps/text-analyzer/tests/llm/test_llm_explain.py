import re

import pytest

from app.llm.explain import build_explanation, in_language, normalise_language
from app.llm.locales import EXPLANATIONS, NEXT_STEPS
from app.llm.schema import Category, ClassificationResult

FORBIDDEN = re.compile(r"100\s*%|scammer|पूरी तरह सुरक्षित|ಸಂಪೂರ್ಣ ಸುರಕ್ಷಿತ", re.I)


def res(expl: str, reasons=(), cat=Category.OTP_PIN, lang="en") -> ClassificationResult:
    return ClassificationResult(
        category=cat,
        is_scam=cat is not Category.LEGITIMATE,
        confidence=0.9,
        reasons=list(reasons),
        explanation=expl,
        language=lang,
    )


@pytest.mark.parametrize("lang", ["en", "hi", "kn"])
def test_every_category_has_template_in_each_language(lang):
    assert set(EXPLANATIONS[lang]) == set(Category)
    for text in [*EXPLANATIONS[lang].values(), *NEXT_STEPS[lang].values()]:
        assert in_language(text, lang), text
        assert not FORBIDDEN.search(text)


def test_llm_explanation_used_when_script_matches():
    e = build_explanation("hi", res("यह संदेश आपका OTP माँग रहा है।", ["OTP माँगा गया"], lang="hi"))
    assert e.source == "llm" and e.reasons == ["OTP माँगा गया"]
    assert "1930" in e.next_steps


def test_template_used_when_llm_answers_in_wrong_script():
    e = build_explanation("kn", res("This asks for your OTP."))
    assert e.source == "template" and in_language(e.summary, "kn")


def test_rules_only_path_uses_template():
    e = build_explanation("hi", None, category=Category.UTILITY_BILL, rule_reasons=["urgency"])
    assert e.source == "template" and e.summary == EXPLANATIONS["hi"][Category.UTILITY_BILL]


def test_legit_wording():
    e = build_explanation("en", None, category=Category.LEGITIMATE)
    assert e.summary.startswith("No red flags found")


def test_language_normalisation():
    assert normalise_language("hi-IN") == "hi"
    assert normalise_language("ta") == "en"
    assert normalise_language(None) == "en"
