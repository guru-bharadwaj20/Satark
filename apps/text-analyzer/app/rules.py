"""YAML-driven scam rule engine.

Rule files live in ``rules/<lang>.yaml`` at the repository root and are
mounted into the container at ``RULES_DIR``. Schema::

    version: 1
    lang: en
    rules:
      - id: urgency.deadline          # unique, dotted: <category>.<name>
        category: urgency
        weight: 20                    # 1-100, contribution to the risk score
        patterns:                     # case-insensitive regexes, ANY may match
          - '\\btonight\\b'
        requires:                     # optional regexes, ALL must also match
          - 'disconnect'
        reason:                       # string, or map of lang -> string
          en: It gives a deadline to rush you
          hi: ...

Rules always run against *redacted* text, so patterns may refer to
placeholders such as ``\\[PHONE\\]``. Every loaded rule runs regardless of
the requested language because scam messages often mix scripts; the
language only selects which ``reason`` text is returned.
"""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

log = logging.getLogger(__name__)

SUPPORTED_LANGS = ("en", "hi", "kn")
_ID_RE = re.compile(r"^[a-z0-9_]+(\.[a-z0-9_]+)+$")


class RuleError(ValueError):
    """Raised when a rule file is invalid."""


@dataclass(frozen=True)
class Rule:
    id: str
    category: str
    weight: int
    patterns: tuple[re.Pattern[str], ...]
    requires: tuple[re.Pattern[str], ...]
    reasons: dict[str, str]
    source: str

    def matches(self, text: str) -> bool:
        if not any(p.search(text) for p in self.patterns):
            return False
        return all(p.search(text) for p in self.requires)

    def reason(self, lang: str) -> str:
        return self.reasons.get(lang) or self.reasons.get("en") or next(iter(self.reasons.values()))


@dataclass(frozen=True)
class Match:
    id: str
    category: str
    weight: int
    reason: str

    def as_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "category": self.category,
            "weight": self.weight,
            "reason": self.reason,
        }


def _compile(raw: Any, where: str) -> tuple[re.Pattern[str], ...]:
    if raw is None:
        return ()
    if not isinstance(raw, list) or not all(isinstance(p, str) for p in raw):
        raise RuleError(f"{where}: expected a list of regex strings")
    try:
        return tuple(re.compile(p, re.IGNORECASE | re.UNICODE) for p in raw)
    except re.error as exc:
        raise RuleError(f"{where}: invalid regex: {exc}") from exc


def _parse_rule(raw: Any, file_lang: str, source: str) -> Rule:
    if not isinstance(raw, dict):
        raise RuleError(f"{source}: each rule must be a mapping")
    rule_id = raw.get("id")
    where = f"{source}:{rule_id}"
    if not isinstance(rule_id, str) or not _ID_RE.match(rule_id):
        raise RuleError(f"{where}: id must be dotted lowercase, e.g. urgency.deadline")
    category = raw.get("category") or rule_id.split(".", 1)[0]
    weight = raw.get("weight")
    if not isinstance(weight, int) or not 1 <= weight <= 100:
        raise RuleError(f"{where}: weight must be an integer 1-100")
    patterns = _compile(raw.get("patterns"), where)
    if not patterns:
        raise RuleError(f"{where}: at least one pattern is required")
    reason = raw.get("reason")
    if isinstance(reason, str):
        reasons = {file_lang: reason}
    elif isinstance(reason, dict) and reason and all(isinstance(v, str) for v in reason.values()):
        reasons = {str(k): v for k, v in reason.items()}
    else:
        raise RuleError(f"{where}: reason must be a string or a lang->string map")
    return Rule(
        id=rule_id,
        category=str(category),
        weight=weight,
        patterns=patterns,
        requires=_compile(raw.get("requires"), where),
        reasons=reasons,
        source=source,
    )


def load_file(path: Path) -> list[Rule]:
    doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(doc, dict) or doc.get("version") != 1:
        raise RuleError(f"{path.name}: expected 'version: 1'")
    lang = doc.get("lang", path.stem)
    if lang not in SUPPORTED_LANGS:
        raise RuleError(f"{path.name}: unsupported lang {lang!r}")
    raw_rules = doc.get("rules") or []
    if not isinstance(raw_rules, list):
        raise RuleError(f"{path.name}: 'rules' must be a list")
    return [_parse_rule(r, lang, path.name) for r in raw_rules]


class RuleEngine:
    def __init__(self, rules: list[Rule]) -> None:
        seen: dict[str, str] = {}
        for r in rules:
            if r.id in seen:
                raise RuleError(f"duplicate rule id {r.id!r} in {r.source} and {seen[r.id]}")
            seen[r.id] = r.source
        self.rules = rules

    @classmethod
    def from_dir(cls, directory: Path) -> RuleEngine:
        rules: list[Rule] = []
        for path in sorted(directory.glob("*.yaml")):
            rules.extend(load_file(path))
        log.info("loaded %d rules from %s", len(rules), directory)
        return cls(rules)

    def __len__(self) -> int:
        return len(self.rules)

    def evaluate(self, text: str, lang: str = "en") -> list[Match]:
        """Return matching rules, highest weight first."""
        hits = [
            Match(r.id, r.category, r.weight, r.reason(lang)) for r in self.rules if r.matches(text)
        ]
        hits.sort(key=lambda m: (-m.weight, m.id))
        return hits
