// Package api exposes the reputation HTTP API.
//
//	POST /v1/reports   record a community report
//	GET  /v1/lookup    aggregate reports for a phone number, UPI ID or domain
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/metrics"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

// Categories a report may carry.
var Categories = map[string]bool{
	"kyc_fraud": true, "upi_collect": true, "fake_job": true, "lottery": true,
	"impersonation": true, "investment": true, "loan_app": true, "other": true,
}

// Config tunes the server.
type Config struct {
	Store   store.Store
	Hasher  *identifier.Hasher
	Metrics *metrics.Registry
	Logger  *slog.Logger
	Now     func() time.Time
}

// Server implements http.Handler.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New builds a Server with defaults filled in.
func New(cfg Config) *Server {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = metrics.New()
	}
	cfg.Metrics.Describe("satark_reputation_reports_total", "Reports received by kind and result.")
	cfg.Metrics.Describe("satark_reputation_lookups_total", "Lookups served by kind.")
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /v1/reports", s.report)
	s.mux.HandleFunc("GET /v1/lookup", s.lookup)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.Handle("GET /metrics", cfg.Metrics)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// ReportRequest is the body of POST /v1/reports. ReporterID is an opaque,
// caller-scoped ID (e.g. "tg:<user id>"); it is hashed before storage.
type ReportRequest struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Category   string `json:"category"`
	ReporterID string `json:"reporter_id"`
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	var req ReportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	kind, norm, ok := s.parse(w, req.Kind, req.Value)
	if !ok {
		return
	}
	if req.Category == "" {
		req.Category = "other"
	}
	if !Categories[req.Category] {
		writeErr(w, http.StatusBadRequest, "unknown category")
		return
	}
	if req.ReporterID == "" || len(req.ReporterID) > 128 {
		writeErr(w, http.StatusBadRequest, "reporter_id is required")
		return
	}
	added, err := s.cfg.Store.AddReport(r.Context(), store.Report{
		Kind:         string(kind),
		SubjectHash:  s.cfg.Hasher.Subject(kind, norm),
		ReporterHash: s.cfg.Hasher.Reporter(req.ReporterID),
		Category:     req.Category,
		CreatedAt:    s.cfg.Now().UTC(),
	})
	if err != nil {
		s.internal(w, "add report", err)
		return
	}
	result := "accepted"
	if !added {
		result = "duplicate"
	}
	s.cfg.Metrics.Inc("satark_reputation_reports_total", "kind", string(kind), "result", result)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": result})
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, norm, ok := s.parse(w, q.Get("kind"), q.Get("value"))
	if !ok {
		return
	}
	sum, err := s.cfg.Store.Summary(r.Context(), string(kind), s.cfg.Hasher.Subject(kind, norm))
	if err != nil {
		s.internal(w, "summary", err)
		return
	}
	s.cfg.Metrics.Inc("satark_reputation_lookups_total", "kind", string(kind))
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":               kind,
		"distinct_reporters": sum.DistinctReporters,
		"categories":         sum.Categories,
	})
}

func (s *Server) parse(w http.ResponseWriter, k, v string) (identifier.Kind, string, bool) {
	kind, err := identifier.ParseKind(k)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "kind must be phone, upi or domain")
		return "", "", false
	}
	norm, err := identifier.Normalize(kind, v)
	if errors.Is(err, identifier.ErrInvalid) {
		writeErr(w, http.StatusBadRequest, "value is not a valid "+string(kind))
		return "", "", false
	}
	return kind, norm, true
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.cfg.Store.Ping(ctx); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	w.Write([]byte("ok"))
}

func (s *Server) internal(w http.ResponseWriter, op string, err error) {
	s.cfg.Logger.Error("store error", "op", op, "err", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
