import os
from pathlib import Path

# Point the service at the repository rule set when it exists, else at a
# minimal fixture, before app.main reads its settings.
_REPO_RULES = Path(__file__).resolve().parents[3] / "rules"
_FIXTURE_RULES = Path(__file__).parent / "fixtures" / "rules"
os.environ.setdefault("RULES_DIR", str(_REPO_RULES if _REPO_RULES.is_dir() else _FIXTURE_RULES))
