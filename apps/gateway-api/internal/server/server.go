// Package server wires the gateway-api HTTP router.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/config"
)

// Server holds dependencies shared by handlers.
type Server struct {
	cfg     config.Config
	log     *slog.Logger
	ready   atomic.Bool
	scanner Scanner
}

// New builds a Server.
func New(cfg config.Config, log *slog.Logger) *Server {
	return &Server{cfg: cfg, log: log}
}

// SetReady toggles the readiness probe (false while draining on shutdown).
func (s *Server) SetReady(v bool) { s.ready.Store(v) }

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.Handle("GET /metrics", promhttp.Handler())
	s.routes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return instrument(s.log, mux)
}

// routes registers the versioned API.
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/scan/text", s.requireScanner(s.scanText))
	mux.HandleFunc("POST /v1/scan/qr", s.requireScanner(s.scanQR))
}

func (s *Server) requireScanner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.scanner == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "scanner not configured")
			return
		}
		next(w, r)
	}
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type ctxKey int

const requestIDKey ctxKey = iota

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the correlation ID stored in ctx, if any.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = msg
	writeJSON(w, status, b)
}
