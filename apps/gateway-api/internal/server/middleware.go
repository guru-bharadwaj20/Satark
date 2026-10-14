package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "satark_gateway_http_requests_total",
		Help: "HTTP requests handled by gateway-api.",
	}, []string{"route", "method", "status"})
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "satark_gateway_http_request_duration_seconds",
		Help:    "HTTP request latency.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"route", "method"})
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// RequestIDHeader carries the request correlation ID.
const RequestIDHeader = "X-Request-Id"

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// instrument wraps a handler with request IDs, panic recovery, structured
// access logs and Prometheus metrics. The route label is the mux pattern,
// so metric cardinality stays bounded.
func instrument(log *slog.Logger, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := r.Header.Get(RequestIDHeader)
		if reqID == "" || len(reqID) > 64 {
			reqID = newRequestID()
		}
		w.Header().Set(RequestIDHeader, reqID)
		rec := &statusRecorder{ResponseWriter: w}

		_, route := mux.Handler(r)
		if route == "" {
			route = "unmatched"
		}

		defer func() {
			if p := recover(); p != nil {
				log.Error("panic", "request_id", reqID, "route", route, "panic", p)
				if rec.status == 0 {
					writeError(rec, http.StatusInternalServerError, "internal", "internal error")
				}
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			elapsed := time.Since(start)
			httpRequests.WithLabelValues(route, r.Method, strconv.Itoa(status)).Inc()
			httpDuration.WithLabelValues(route, r.Method).Observe(elapsed.Seconds())
			level := slog.LevelInfo
			if route == "GET /healthz" || route == "GET /readyz" || route == "GET /metrics" {
				level = slog.LevelDebug
			}
			log.Log(r.Context(), level, "request",
				"request_id", reqID,
				"method", r.Method,
				"route", route,
				"status", status,
				"bytes", rec.bytes,
				"duration_ms", elapsed.Milliseconds(),
			)
		}()
		mux.ServeHTTP(rec, r.WithContext(withRequestID(r.Context(), reqID)))
	})
}
