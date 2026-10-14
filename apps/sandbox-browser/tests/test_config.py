import pytest

from app import config


def test_load_valid():
    env = {"TARGET_URL": "https://example.com/x", "SCAN_ID": "abc", "TIMEOUT_SECONDS": "30"}
    s = config.load(env)
    assert s.target_url == "https://example.com/x"
    assert s.scan_id == "abc"
    assert s.navigation_budget_ms == 22_000


@pytest.mark.parametrize("url", ["", "file:///etc/passwd", "javascript:alert(1)", "example.com"])
def test_rejects_bad_urls(url):
    with pytest.raises(config.ConfigError):
        config.load({"TARGET_URL": url})


@pytest.mark.parametrize("timeout", ["5", "301", "abc"])
def test_rejects_bad_timeout(timeout):
    with pytest.raises(config.ConfigError):
        config.load({"TARGET_URL": "https://example.com", "TIMEOUT_SECONDS": timeout})
