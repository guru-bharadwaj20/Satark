"""Per-alert cooldown and a daily LLM token budget.

opsbot runs as a single replica, so in-memory state is sufficient; a restart
resets the counters, which errs on the side of posting.
"""

from __future__ import annotations

import time
from datetime import UTC, datetime

from prometheus_client import Gauge

BUDGET_REMAINING = Gauge("satark_opsbot_token_budget_remaining", "Tokens left in today's budget.")


class Cooldown:
    def __init__(self, seconds: int, clock=time.monotonic, max_keys: int = 5000) -> None:
        self.seconds, self._clock, self.max_keys = seconds, clock, max_keys
        self._last: dict[str, float] = {}

    def remaining(self, key: str) -> float:
        last = self._last.get(key)
        if last is None:
            return 0.0
        return max(0.0, self.seconds - (self._clock() - last))

    def try_acquire(self, key: str) -> bool:
        """True if the key is not cooling down; marks it as just seen."""
        if self.remaining(key) > 0:
            return False
        if len(self._last) >= self.max_keys:
            now = self._clock()
            self._last = {k: t for k, t in self._last.items() if now - t < self.seconds}
        self._last[key] = self._clock()
        return True


class DailyTokenBudget:
    def __init__(self, limit: int, now=lambda: datetime.now(UTC)) -> None:
        self.limit, self._now = limit, now
        self._day = self._now().date()
        self.used = 0
        BUDGET_REMAINING.set(limit)

    def _roll(self) -> None:
        today = self._now().date()
        if today != self._day:
            self._day, self.used = today, 0

    @property
    def remaining(self) -> int:
        self._roll()
        return max(0, self.limit - self.used)

    def allows(self, estimate: int = 2000) -> bool:
        return self.remaining >= estimate

    def record(self, tokens: int) -> None:
        self._roll()
        self.used += max(0, tokens)
        BUDGET_REMAINING.set(self.remaining)
