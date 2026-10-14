"""Build the user-facing explanation in English, Hindi or Kannada."""

from __future__ import annotations

import unicodedata
from dataclasses import dataclass

from app.llm.locales import EXPLANATIONS, NEXT_STEPS
from app.llm.schema import Category, ClassificationResult

SUPPORTED = ("en", "hi", "kn")

# Unicode blocks for script checks.
_SCRIPTS = {"hi": (0x0900, 0x097F), "kn": (0x0C80, 0x0CFF)}


def script_ratio(text: str, language: str) -> float:
    """Share of letters in ``text`` that belong to ``language``'s script."""
    # Include combining marks so Indic vowel signs count towards their script.
    letters = [ch for ch in text if ch.isalpha() or unicodedata.category(ch).startswith("M")]
    if not letters:
        return 0.0
    if language == "en":
        return sum(ch.isascii() for ch in letters) / len(letters)
    lo, hi = _SCRIPTS[language]
    return sum(lo <= ord(ch) <= hi for ch in letters) / len(letters)


def in_language(text: str, language: str, threshold: float = 0.6) -> bool:
    return script_ratio(text, language) >= threshold


@dataclass(frozen=True, slots=True)
class Explanation:
    language: str
    summary: str
    reasons: list[str]
    next_steps: str
    source: str  # "llm" or "template"


def normalise_language(language: str | None) -> str:
    lang = (language or "en").lower().split("-")[0]
    return lang if lang in SUPPORTED else "en"


def build_explanation(
    language: str | None,
    llm: ClassificationResult | None = None,
    *,
    category: Category | None = None,
    rule_reasons: list[str] | None = None,
) -> Explanation:
    """Prefer the LLM's explanation when it is in the right script; otherwise
    use the reviewed template for the category (from the LLM or the rules)."""
    lang = normalise_language(language)
    cat = llm.category if llm is not None else (category or Category.OTHER_SCAM)
    is_scam = cat is not Category.LEGITIMATE
    steps = NEXT_STEPS[lang]["scam" if is_scam else "legit"]

    if llm is not None and in_language(llm.explanation, lang):
        reasons = [r for r in llm.reasons if in_language(r, lang)] or list(rule_reasons or [])
        return Explanation(lang, llm.explanation, reasons, steps, "llm")
    return Explanation(lang, EXPLANATIONS[lang][cat], list(rule_reasons or []), steps, "template")
