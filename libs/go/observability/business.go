package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Allowed label values. Anything else is recorded as "other" so a buggy
// caller can never explode series cardinality.
var (
	Verdicts   = []string{"low_risk", "suspicious", "high_risk", "unknown"}
	Categories = []string{"phishing", "lottery_prize", "kyc_update", "job_offer", "investment", "loan_app", "digital_arrest", "sextortion", "courier_customs", "upi_collect", "impersonation", "malware_apk", "none"}
	Channels   = []string{"pwa", "telegram", "api"}
	ScanKinds  = []string{"url", "text", "qr", "image"}
	LLMOutcome = []string{"ok", "error", "timeout", "skipped"}
	CacheHits  = []string{"hit", "miss", "error"}
)

// BusinessMetrics are the product-level counters behind the usage and scan
// pipeline dashboards.
//
//	satark_scans_total{service,kind,verdict,category,channel}
//	satark_llm_calls_total{service,provider,model,outcome}
//	satark_llm_call_duration_seconds{service,provider}
//	satark_llm_tokens_total{service,provider,direction}
//	satark_cache_requests_total{service,cache,result}
type BusinessMetrics struct {
	scans       *prometheus.CounterVec
	llmCalls    *prometheus.CounterVec
	llmDuration *prometheus.HistogramVec
	llmTokens   *prometheus.CounterVec
	cache       *prometheus.CounterVec
}

// NewBusinessMetrics registers business metrics for service on reg.
func NewBusinessMetrics(reg prometheus.Registerer, service string) *BusinessMetrics {
	cl := prometheus.Labels{"service": service}
	b := &BusinessMetrics{
		scans: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "satark_scans_total", Help: "Completed scans by kind, verdict, scam category and channel.", ConstLabels: cl,
		}, []string{"kind", "verdict", "category", "channel"}),
		llmCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "satark_llm_calls_total", Help: "LLM calls by provider, model and outcome.", ConstLabels: cl,
		}, []string{"provider", "model", "outcome"}),
		llmDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "satark_llm_call_duration_seconds", Help: "LLM call latency in seconds.", ConstLabels: cl,
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30},
		}, []string{"provider"}),
		llmTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "satark_llm_tokens_total", Help: "LLM tokens consumed, by direction (input/output).", ConstLabels: cl,
		}, []string{"provider", "direction"}),
		cache: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "satark_cache_requests_total", Help: "Cache lookups by cache name and result (hit/miss/error).", ConstLabels: cl,
		}, []string{"cache", "result"}),
	}
	reg.MustRegister(b.scans, b.llmCalls, b.llmDuration, b.llmTokens, b.cache)
	return b
}

// Scan records one completed scan.
func (b *BusinessMetrics) Scan(kind, verdict, category, channel string) {
	b.scans.WithLabelValues(oneOf(kind, ScanKinds), oneOf(verdict, Verdicts), oneOf(category, Categories), oneOf(channel, Channels)).Inc()
}

// LLMCall records one LLM call and its token usage.
func (b *BusinessMetrics) LLMCall(provider, model, outcome string, elapsed time.Duration, inTokens, outTokens int) {
	b.llmCalls.WithLabelValues(provider, model, oneOf(outcome, LLMOutcome)).Inc()
	if outcome != "skipped" {
		b.llmDuration.WithLabelValues(provider).Observe(elapsed.Seconds())
	}
	if inTokens > 0 {
		b.llmTokens.WithLabelValues(provider, "input").Add(float64(inTokens))
	}
	if outTokens > 0 {
		b.llmTokens.WithLabelValues(provider, "output").Add(float64(outTokens))
	}
}

// Cache records one cache lookup; cache is a short static name such as "url_intel".
func (b *BusinessMetrics) Cache(cache, result string) {
	b.cache.WithLabelValues(cache, oneOf(result, CacheHits)).Inc()
}

func oneOf(v string, allowed []string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return "other"
}
