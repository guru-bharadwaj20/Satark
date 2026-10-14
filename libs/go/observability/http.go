// Package observability provides the shared RED metrics, business metrics and
// structured JSON logging used by every Satark Go service.
//
// All metric names carry the satark_ prefix and a constant "service" label so
// dashboards and SLO rules can be written once for every service.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// DefaultBuckets are latency buckets (seconds) tuned for an API whose p95
// objective is 800ms: dense around 0.1–2s, with a long tail for LLM calls.
var DefaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.8, 1, 2, 5, 10}

// HTTPMetrics holds the RED (rate, errors, duration) collectors for one service.
type HTTPMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// NewRegistry returns a registry pre-loaded with Go runtime and process
// collectors, labelled with the service name.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

// NewHTTPMetrics registers RED metrics for service on reg.
//
// Exposed series:
//
//	satark_http_requests_total{service,method,route,code}
//	satark_http_request_duration_seconds{service,method,route}
//	satark_http_requests_in_flight{service}
func NewHTTPMetrics(reg *prometheus.Registry, service string) *HTTPMetrics {
	constLabels := prometheus.Labels{"service": service}
	m := &HTTPMetrics{
		registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "satark_http_requests_total",
			Help:        "HTTP requests handled, by method, route template and status code.",
			ConstLabels: constLabels,
		}, []string{"method", "route", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "satark_http_request_duration_seconds",
			Help:        "HTTP request latency in seconds, by method and route template.",
			ConstLabels: constLabels,
			Buckets:     DefaultBuckets,
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "satark_http_requests_in_flight",
			Help:        "HTTP requests currently being served.",
			ConstLabels: constLabels,
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.inFlight)
	return m
}

// Middleware instruments next. route must be a low-cardinality route template
// (e.g. "/v1/scan/url"), never the raw request path.
func (m *HTTPMetrics) Middleware(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inFlight.Inc()
		defer m.inFlight.Dec()
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			// Count panics as 500s before re-raising them.
			if rec := recover(); rec != nil {
				rw.status = http.StatusInternalServerError
				m.observe(r.Method, route, rw.status, start)
				panic(rec)
			}
			m.observe(r.Method, route, rw.status, start)
		}()
		next.ServeHTTP(rw, r)
	})
}

// Observe records one request manually, for routers that resolve the route
// template only after matching (e.g. chi's RoutePattern).
func (m *HTTPMetrics) Observe(method, route string, status int, elapsed time.Duration) {
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, route).Observe(elapsed.Seconds())
}

func (m *HTTPMetrics) observe(method, route string, status int, start time.Time) {
	m.Observe(method, route, status, time.Since(start))
}

// Handler serves the registry in Prometheus/OpenMetrics format on /metrics.
func (m *HTTPMetrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
