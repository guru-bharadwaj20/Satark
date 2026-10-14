import pytest
from prometheus_client import CollectorRegistry

from satark_obs import BusinessMetrics


def get(registry, name, **labels):
    return registry.get_sample_value(name, {"service": "text-analyzer", **labels})


def test_scan_counts_and_clamps():
    reg = CollectorRegistry()
    biz = BusinessMetrics("text-analyzer", reg)
    biz.scan("text", "suspicious", "job_offer", "pwa")
    biz.scan("text", "100%-safe", "?", "sms")
    assert get(reg, "satark_scans_total", kind="text", verdict="suspicious", category="job_offer", channel="pwa") == 1
    assert get(reg, "satark_scans_total", kind="text", verdict="other", category="other", channel="other") == 1


def test_time_llm_success_and_error():
    reg = CollectorRegistry()
    biz = BusinessMetrics("text-analyzer", reg)
    with biz.time_llm("anthropic", "haiku") as ctx:
        ctx["in_tokens"], ctx["out_tokens"] = 300, 40
    with pytest.raises(TimeoutError):
        with biz.time_llm("anthropic", "haiku"):
            raise TimeoutError
    assert get(reg, "satark_llm_calls_total", provider="anthropic", model="haiku", outcome="ok") == 1
    assert get(reg, "satark_llm_calls_total", provider="anthropic", model="haiku", outcome="timeout") == 1
    assert get(reg, "satark_llm_tokens_total", provider="anthropic", direction="input") == 300
    assert get(reg, "satark_llm_call_duration_seconds_count", provider="anthropic") == 2


def test_cache_lookup():
    reg = CollectorRegistry()
    biz = BusinessMetrics("text-analyzer", reg)
    biz.cache_lookup("verdict", "hit")
    biz.cache_lookup("verdict", "bogus")
    assert get(reg, "satark_cache_requests_total", cache="verdict", result="hit") == 1
    assert get(reg, "satark_cache_requests_total", cache="verdict", result="other") == 1
