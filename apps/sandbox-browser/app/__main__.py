"""Entry point: ``python -m app``. Exit 0 on a completed scan, 1 otherwise."""

from __future__ import annotations

import asyncio
import logging
import sys

from . import config
from .report import ScanReport, encode

log = logging.getLogger("sandbox-browser")


def write_result(path: str, report: ScanReport) -> None:
    payload = encode(report)
    try:
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(payload)
    except OSError:
        log.exception("could not write result to %s", path)
    # Also log it so `kubectl logs` and FallbackToLogsOnError see it.
    print(payload, flush=True)


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        stream=sys.stderr,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    try:
        settings = config.load()
    except config.ConfigError as exc:
        write_result("/dev/termination-log", ScanReport(error=f"config: {exc}"))
        return 1

    from .browser import scan  # imported late so config errors need no browser

    try:
        report = asyncio.run(asyncio.wait_for(scan(settings), settings.timeout_seconds - 3))
    except TimeoutError:
        report = ScanReport(error="scan timed out")
    except Exception as exc:  # every failure must produce a result
        log.exception("scan crashed")
        report = ScanReport(error=f"crash: {type(exc).__name__}: {exc}")
    write_result(settings.result_path, report)
    return 1 if report.error and not report.final_url else 0


if __name__ == "__main__":
    sys.exit(main())
