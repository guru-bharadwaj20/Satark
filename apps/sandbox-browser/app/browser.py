"""Drive headless Chromium against the target URL."""

from __future__ import annotations

import logging

from playwright.async_api import Error as PlaywrightError
from playwright.async_api import async_playwright

from .config import Settings
from .report import ScanReport

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


async def scan(settings: Settings) -> ScanReport:
    report = ScanReport()
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
            try:
                await page.goto(
                    settings.target_url,
                    wait_until="load",
                    timeout=settings.navigation_budget_ms,
                )
            except PlaywrightError as exc:
                report.error = str(exc).splitlines()[0]
            report.final_url = page.url
            try:
                report.title = await page.title()
            except PlaywrightError:
                pass
            await context.close()
        finally:
            await browser.close()
    return report
