"""Detect file downloads the page triggers, with special attention to APKs.

Downloads are never saved (the browser context has ``accept_downloads=False``);
only the file name is recorded. Detection uses both Playwright ``download``
events and response headers, because some pages stream an APK with a
``Content-Disposition: attachment`` header that never surfaces as a download
in a headless browser.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from urllib.parse import unquote, urlparse

RISKY_EXTENSIONS = (
    ".apk", ".xapk", ".apks", ".aab", ".exe", ".msi", ".bat", ".cmd", ".scr",
    ".dmg", ".pkg", ".jar", ".js", ".vbs", ".ps1", ".zip", ".rar", ".7z",
)  # fmt: skip
RISKY_MIME = {
    "application/vnd.android.package-archive",
    "application/x-msdownload",
    "application/x-msdos-program",
    "application/java-archive",
    "application/octet-stream",
    "application/zip",
    "application/x-rar-compressed",
}
MAX_DOWNLOADS = 10

_FILENAME_RE = re.compile(r"""filename\*?=(?:UTF-8'')?["']?([^"';]+)""", re.IGNORECASE)


def filename_from_url(url: str) -> str:
    path = unquote(urlparse(url).path)
    return path.rsplit("/", 1)[-1] or "download"


def filename_from_disposition(value: str) -> str | None:
    m = _FILENAME_RE.search(value or "")
    return unquote(m.group(1).strip()) if m else None


def is_risky_name(name: str) -> bool:
    return name.lower().endswith(RISKY_EXTENSIONS)


@dataclass
class DownloadWatcher:
    files: list[str] = field(default_factory=list)

    def _add(self, name: str) -> None:
        name = name.strip()[:200]
        if name and name not in self.files and len(self.files) < MAX_DOWNLOADS:
            self.files.append(name)

    def on_download(self, suggested_filename: str, url: str) -> None:
        self._add(suggested_filename or filename_from_url(url))

    def on_response(self, url: str, headers: dict[str, str]) -> None:
        h = {k.lower(): v for k, v in headers.items()}
        disposition = h.get("content-disposition", "")
        mime = h.get("content-type", "").split(";", 1)[0].strip().lower()
        name = filename_from_disposition(disposition) or filename_from_url(url)
        if disposition.lower().startswith("attachment"):
            self._add(name)
        elif mime == "application/vnd.android.package-archive" or (
            mime in RISKY_MIME and is_risky_name(name)
        ):
            self._add(name)
        elif is_risky_name(name) and name.lower().endswith((".apk", ".xapk", ".apks")):
            self._add(name)

    @property
    def has_apk(self) -> bool:
        return any(f.lower().endswith((".apk", ".xapk", ".apks", ".aab")) for f in self.files)
