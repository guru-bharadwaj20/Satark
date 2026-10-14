from datetime import UTC, datetime

import pytest

from app.storage import screenshot_key, upload_screenshot

NOW = datetime(2026, 10, 14, 9, 30, tzinfo=UTC)


class FakeS3:
    def __init__(self):
        self.calls = []

    def put_object(self, **kwargs):
        self.calls.append(kwargs)


def test_key_layout_and_sanitising():
    assert screenshot_key("scan-1", NOW) == "screenshots/2026/10/14/scan-1.png"
    assert screenshot_key("../../etc", NOW) == "screenshots/2026/10/14/..-..-etc.png"


def test_upload_puts_encrypted_png():
    s3 = FakeS3()
    key = upload_screenshot(b"\x89PNG", "scan-1", bucket="satark-screens", client=s3, now=NOW)
    assert key == "screenshots/2026/10/14/scan-1.png"
    call = s3.calls[0]
    assert call["Bucket"] == "satark-screens"
    assert call["ContentType"] == "image/png"
    assert call["ServerSideEncryption"] == "AES256"


def test_upload_disabled_without_bucket():
    s3 = FakeS3()
    assert upload_screenshot(b"x", "scan-1", bucket="", client=s3) == ""
    assert s3.calls == []


def test_rejects_huge_screenshot():
    with pytest.raises(ValueError):
        upload_screenshot(b"x" * (6 * 1024 * 1024), "s", bucket="b", client=FakeS3())


def test_moto_roundtrip():
    moto = pytest.importorskip("moto")
    import boto3

    with moto.mock_aws():
        s3 = boto3.client("s3", region_name="us-east-1")
        s3.create_bucket(Bucket="satark-screens")
        key = upload_screenshot(b"\x89PNG", "scan-9", bucket="satark-screens", client=s3, now=NOW)
        body = s3.get_object(Bucket="satark-screens", Key=key)["Body"].read()
        assert body == b"\x89PNG"
