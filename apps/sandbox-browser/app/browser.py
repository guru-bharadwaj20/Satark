"""Drive headless Chromium against the target URL."""

from __future__ import annotations

import logging

from playwright.async_api import Error as PlaywrightError
from playwright.async_api import Frame, Page, Response, async_playwright

from . import forms
from .config import Settings
from .downloads import DownloadWatcher
from .redirects import RedirectTracker
from .report import ScanReport
from .storage import upload_screenshot

log = logging.getLogger(__name__)

# gVisor (runsc) is the isolation boundary; Chromium's own setuid/namespace
# sandbox cannot start inside an unprivileged, capability-less container.
CHROMIUM_ARGS = [
    "--no-sandbox",
    "--disable-dev-shm-usage",
    "--disable-gpu",
    "--disable-extensions",
    "--disable-background-networking",
    "--disable-sync",
    "--metrics-recording-only",
    "--no-first-run",
    "--mute-audio",
]

# Time to let client-side (JS / meta refresh) redirects fire after "load".
SETTLE_MS = 3000


def _attach_redirect_tracking(page: Page, tracker: RedirectTracker) -> None:
    def on_response(resp: Response) -> None:
        req = resp.request
        if req.is_navigation_request() and req.frame == page.main_frame:
            tracker.on_navigation_response(resp.url, resp.status)

    def on_navigated(frame: Frame) -> None:
        if frame == page.main_frame:
            tracker.on_frame_navigated(frame.url)

    page.on("response", on_response)
    page.on("framenavigated", on_navigated)


def _attach_download_tracking(page: Page, watcher: DownloadWatcher) -> None:
    def on_download(download) -> None:
        watcher.on_download(download.suggested_filename, download.url)

    def on_response(resp: Response) -> None:
        watcher.on_response(resp.url, resp.headers)

    page.on("download", on_download)
    page.on("response", on_response)


async def _screenshot(page: Page, scan_id: str) -> str:
    """Capture the viewport and upload it; failures never fail the scan."""
    try:
        png = await page.screenshot(type="png", full_page=False, timeout=5000)
        return upload_screenshot(png, scan_id)
    except Exception as exc:  # screenshots are best effort
        log.warning("screenshot failed: %s", exc)
        return ""


async def scan(settings: Settings) -> ScanReport:
    report = ScanReport()
    tracker = RedirectTracker(settings.target_url)
    watcher = DownloadWatcher()
    async with async_playwright() as pw:
        browser = await pw.chromium.launch(headless=True, args=CHROMIUM_ARGS)
        try:
            context = await browser.new_context(
                viewport={"width": settings.viewport_width, "height": settings.viewport_height},
                user_agent=settings.user_agent,
                java_script_enabled=True,
                accept_downloads=False,
                ignore_https_errors=True,
                service_workers="block",
                permissions=[],
            )
            page = await context.new_page()
            _attach_redirect_tracking(page, tracker)
            _attach_download_tracking(page, watcher)
            try:
                await page.goto(
                    settings.target_url,
                    wait_until="load",
                    timeout=settings.navigation_budget_ms,
                )
                # Give JS and meta-refresh redirects a chance to run.
                try:
                    await page.wait_for_load_state("networkidle", timeout=SETTLE_MS)
                except PlaywrightError:
                    pass
                await page.wait_for_timeout(500)
            except PlaywrightError as exc:
                # Navigating straight to a file aborts with "Download is starting".
                if "Download is starting" not in str(exc):
                    report.error = str(exc).splitlines()[0]
            report.final_url = page.url if page.url != "about:blank" else tracker.final_url
            report.redirect_chain = tracker.chain()
            if report.redirect_chain[-1] != report.final_url:
                report.redirect_chain.append(report.final_url)
            try:
                report.title = await page.title()
            except PlaywrightError:
                pass
            report.forms = await forms.detect(page)
            report.downloads = list(watcher.files)
            report.screenshot_key = await _screenshot(page, settings.scan_id)
            await context.close()
        finally:
            await browser.close()
    return report
