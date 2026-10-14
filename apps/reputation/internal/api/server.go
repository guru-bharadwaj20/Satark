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
	"sort"
	"strconv"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/metrics"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/ratelimit"
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
	// MinReporters is the distinct-reporter threshold below which a
	// lookup reveals nothing (default 3).
	MinReporters int
	// Limiter caps reports per reporter; nil disables limiting.
	Limiter *ratelimit.Limiter
	// AdminToken protects /v1/admin/*; empty disables those endpoints.
	AdminToken string
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
	if cfg.MinReporters < 2 {
		cfg.MinReporters = 3
	}
	if cfg.Metrics == nil {
		cfg.Metrics = metrics.New()
	}
	cfg.Metrics.Describe("satark_reputation_reports_total", "Reports received by kind and result.")
	cfg.Metrics.Describe("satark_reputation_lookups_total", "Lookups served by kind.")
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /v1/reports", s.report)
	s.mux.HandleFunc("GET /v1/lookup", s.lookup)
	s.registerDelisting()
	s.mux.HandleFunc("POST /v1/forget", s.forget)
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
	reporter := s.cfg.Hasher.Reporter(req.ReporterID)
	if s.cfg.Limiter != nil {
		if ok, wait := s.cfg.Limiter.Allow(reporter); !ok {
			s.cfg.Metrics.Inc("satark_reputation_reports_total", "kind", string(kind), "result", "rate_limited")
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			writeErr(w, http.StatusTooManyRequests, "too many reports from this reporter; try again later")
			return
		}
	}
	added, err := s.cfg.Store.AddReport(r.Context(), store.Report{
		Kind:         string(kind),
		SubjectHash:  s.cfg.Hasher.Subject(kind, norm),
		ReporterHash: reporter,
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

// forget deletes every report filed by a reporter. It is called by the
// bot's /forget command; the caller is trusted (cluster-internal, guarded
// by NetworkPolicy) and passes the same reporter_id it reports with.
func (s *Server) forget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReporterID string `json:"reporter_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.ReporterID == "" {
		writeErr(w, http.StatusBadRequest, "reporter_id is required")
		return
	}
	n, err := s.cfg.Store.DeleteReporter(r.Context(), s.cfg.Hasher.Reporter(req.ReporterID))
	if err != nil {
		s.internal(w, "delete reporter", err)
		return
	}
	s.cfg.Metrics.Inc("satark_reputation_forget_requests_total")
	writeJSON(w, http.StatusOK, map[string]any{"reports_deleted": n})
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
	writeJSON(w, http.StatusOK, s.present(kind, norm, sum))
}

// LookupResponse is what callers see. Counts below the threshold are
// never revealed, counts above it are bucketed, and the subject is masked.
// The wording is deliberately factual: it reports how many independent
// people flagged the subject and never labels anyone a scammer.
type LookupResponse struct {
	Kind       identifier.Kind `json:"kind"`
	Subject    string          `json:"subject"`
	Status     string          `json:"status"` // "insufficient_reports" | "reported"
	Reporters  string          `json:"reporters,omitempty"`
	Categories []string        `json:"categories,omitempty"`
	FirstSeen  string          `json:"first_seen,omitempty"`
	LastSeen   string          `json:"last_seen,omitempty"`
	Message    string          `json:"message"`
}

func (s *Server) present(kind identifier.Kind, norm string, sum store.Summary) LookupResponse {
	resp := LookupResponse{Kind: kind, Subject: identifier.Mask(kind, norm)}
	if sum.DistinctReporters < s.cfg.MinReporters {
		resp.Status = "insufficient_reports"
		resp.Message = "Not enough independent reports to show anything. This does not mean it is safe; stay careful."
		return resp
	}
	resp.Status = "reported"
	resp.Reporters = Bucket(sum.DistinctReporters)
	for c, n := range sum.Categories {
		// Only show categories that themselves meet the threshold so a
		// single reporter cannot attach an arbitrary label.
		if n >= s.cfg.MinReporters {
			resp.Categories = append(resp.Categories, c)
		}
	}
	sort.Strings(resp.Categories)
	resp.FirstSeen = sum.FirstSeen.Format("2006-01")
	resp.LastSeen = sum.LastSeen.Format("2006-01")
	resp.Message = "Reported by " + resp.Reporters + " independent people. Be cautious and verify through an official channel."
	return resp
}

// Bucket coarsens a reporter count so exact numbers are not exposed.
func Bucket(n int) string {
	switch {
	case n >= 100:
		return "100+"
	case n >= 50:
		return "50-99"
	case n >= 10:
		return "10-49"
	default:
		return "fewer than 10"
	}
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
