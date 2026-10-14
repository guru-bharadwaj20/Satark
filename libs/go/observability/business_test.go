package observability

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBusinessMetrics(t *testing.T) {
	b := NewBusinessMetrics(NewRegistry(), "gateway-api")

	b.Scan("url", "high_risk", "kyc_update", "telegram")
	b.Scan("url", "high_risk", "kyc_update", "telegram")
	b.Scan("text", "made-up", "whatever", "sms")
	if got := testutil.ToFloat64(b.scans.WithLabelValues("url", "high_risk", "kyc_update", "telegram")); got != 2 {
		t.Fatalf("scans = %v, want 2", got)
	}
	if got := testutil.ToFloat64(b.scans.WithLabelValues("text", "other", "other", "other")); got != 1 {
		t.Fatalf("unknown labels not clamped to other: %v", got)
	}

	b.LLMCall("anthropic", "haiku", "ok", 900*time.Millisecond, 400, 50)
	b.LLMCall("anthropic", "haiku", "skipped", 0, 0, 0)
	if got := testutil.ToFloat64(b.llmTokens.WithLabelValues("anthropic", "input")); got != 400 {
		t.Fatalf("input tokens = %v", got)
	}
	if got := testutil.CollectAndCount(b.llmDuration); got != 1 {
		t.Fatalf("duration series = %d", got)
	}

	b.Cache("url_intel", "hit")
	b.Cache("url_intel", "miss")
	if got := testutil.ToFloat64(b.cache.WithLabelValues("url_intel", "hit")); got != 1 {
		t.Fatalf("cache hits = %v", got)
	}
}
