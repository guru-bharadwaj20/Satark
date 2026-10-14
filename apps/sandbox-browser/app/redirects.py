"""Record the redirect chain of the main frame.

Two kinds of hops are tracked:

* HTTP redirects (3xx with ``Location``), seen as main-frame navigation
  requests and through ``request.redirected_from``;
* client-side redirects (``location.href``, ``<meta http-equiv=refresh>``),
  seen as ``framenavigated`` events on the main frame.
"""

from __future__ import annotations

from dataclasses import dataclass, field

MAX_HOPS = 30


@dataclass
class Hop:
    url: str
    status: int | None = None
    kind: str = "http"  # "http" or "client"


@dataclass
class RedirectTracker:
    start_url: str
    hops: list[Hop] = field(default_factory=list)

    def __post_init__(self) -> None:
        self.hops.append(Hop(self.start_url, None, "start"))

    def _add(self, hop: Hop) -> None:
        if hop.url in ("", "about:blank") or len(self.hops) >= MAX_HOPS:
            return
        last = self.hops[-1]
        if _same(last.url, hop.url):
            # Same URL seen again (e.g. response after request): keep the status.
            if last.status is None:
                last.status = hop.status
            return
        self.hops.append(hop)

    def on_navigation_response(self, url: str, status: int) -> None:
        """A main-frame document response (including 3xx hops)."""
        self._add(Hop(url, status, "http"))

    def on_frame_navigated(self, url: str) -> None:
        """The main frame committed a new document."""
        self._add(Hop(url, None, "client"))

    def chain(self) -> list[str]:
        return [h.url for h in self.hops]

    @property
    def final_url(self) -> str:
        return self.hops[-1].url

    @property
    def hop_count(self) -> int:
        return len(self.hops) - 1


def _same(a: str, b: str) -> bool:
    """Compare URLs ignoring a trailing slash on a bare origin and the fragment."""

    def norm(u: str) -> str:
        u = u.split("#", 1)[0]
        return u[:-1] if u.endswith("/") and u.count("/") == 3 else u

    return norm(a) == norm(b)
