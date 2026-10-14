// Package server exposes url-intel over HTTP.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

// maxBodyBytes bounds request bodies; a URL never needs more.
const maxBodyBytes = 16 << 10

// Analyzer is the subset of analyzer.Analyzer used by the server.
type Analyzer interface {
	Analyze(ctx context.Context, raw string) (*analyzer.Report, error)
}

// ReadyFunc reports whether dependencies are reachable.
type ReadyFunc func(ctx context.Context) error

// Server is the HTTP front of url-intel.
type Server struct {
	an       Analyzer
	ready    ReadyFunc
	timeout  time.Duration
	log      *slog.Logger
	draining atomic.Bool

	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	signals  *prometheus.CounterVec
	registry *prometheus.Registry
}

// New creates a Server. ready may be nil.
func New(an Analyzer, ready ReadyFunc, timeout time.Duration, log *slog.Logger) *Server {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	s := &Server{
		an: an, ready: ready, timeout: timeout, log: log, registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "satark_url_intel_requests_total", Help: "Analyze requests by HTTP status code.",
		}, []string{"code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "satark_url_intel_request_duration_seconds", Help: "Analyze request latency.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20},
		}, []string{"code"}),
		signals: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "satark_url_intel_signals_total", Help: "Signals emitted by id.",
		}, []string{"id"}),
	}
	reg.MustRegister(s.requests, s.duration, s.signals)
	return s
}

// Registry exposes the metrics registry so other components can register.
func (s *Server) Registry() *prometheus.Registry { return s.registry }

// SetDraining marks the server as shutting down so /readyz fails.
func (s *Server) SetDraining() { s.draining.Store(true) }

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("POST /v1/analyze", s.handleAnalyze)
	return mux
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "draining"})
		return
	}
	if s.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.ready(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready", "error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

type analyzeRequest struct {
	URL string `json:"url"`
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	code := http.StatusOK
	defer func() {
		c := strconv.Itoa(code)
		s.requests.WithLabelValues(c).Inc()
		s.duration.WithLabelValues(c).Observe(time.Since(start).Seconds())
	}()

	var req analyzeRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		code = http.StatusBadRequest
		writeJSON(w, code, map[string]string{"error": `body must be JSON with a non-empty "url" field`})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	rep, err := s.an.Analyze(ctx, req.URL)
	if err != nil {
		if errors.Is(err, analyzer.ErrInvalidInput) {
			code = http.StatusUnprocessableEntity
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		code = http.StatusInternalServerError
		s.log.Error("analyze failed", "err", err)
		writeJSON(w, code, map[string]string{"error": "analysis failed"})
		return
	}
	for _, sig := range rep.Signals {
		s.signals.WithLabelValues(sig.ID).Inc()
	}
	writeJSON(w, code, rep)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
