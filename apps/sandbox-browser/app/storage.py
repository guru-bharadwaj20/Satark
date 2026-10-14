"""Upload sandbox screenshots to S3.

Credentials come from the environment (``AWS_ACCESS_KEY_ID`` etc., injected
from a Secret scoped to ``s3:PutObject`` on the ``screenshots/`` prefix).
Uploads are skipped when ``SCREENSHOT_BUCKET`` is unset.
"""

from __future__ import annotations

import logging
import os
import re
from datetime import UTC, datetime

log = logging.getLogger(__name__)

PREFIX = "screenshots"
MAX_BYTES = 5 * 1024 * 1024
_SAFE_ID = re.compile(r"[^a-zA-Z0-9._-]")


def screenshot_key(scan_id: str, now: datetime | None = None) -> str:
    now = now or datetime.now(UTC)
    safe = _SAFE_ID.sub("-", scan_id)[:100] or "scan"
    return f"{PREFIX}/{now:%Y/%m/%d}/{safe}.png"


def upload_screenshot(
    png: bytes,
    scan_id: str,
    bucket: str | None = None,
    client=None,
    now: datetime | None = None,
) -> str:
    """Upload ``png`` and return its object key, or "" if upload is disabled."""
    bucket = bucket if bucket is not None else os.environ.get("SCREENSHOT_BUCKET", "")
    if not bucket or not png:
        return ""
    if len(png) > MAX_BYTES:
        raise ValueError(f"screenshot too large ({len(png)} bytes)")
    if client is None:
        import boto3
        from botocore.config import Config

        client = boto3.client(
            "s3",
            region_name=os.environ.get("AWS_REGION", "ap-south-1"),
            config=Config(retries={"max_attempts": 3, "mode": "standard"}, connect_timeout=5),
        )
    key = screenshot_key(scan_id, now)
    client.put_object(
        Bucket=bucket,
        Key=key,
        Body=png,
        ContentType="image/png",
        ServerSideEncryption="AES256",
        Tagging="source=sandbox-browser",
    )
    log.info("uploaded screenshot s3://%s/%s", bucket, key)
    return key
