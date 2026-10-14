package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/config"
)

func newTestServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetReady(true)
	return s
}

func TestHealthAndReady(t *testing.T) {
	s := newTestServer(t, config.Config{})
	h := s.Handler()

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d", path, rec.Code)
		}
		if rec.Header().Get(RequestIDHeader) == "" {
			t.Fatalf("%s: missing request id", path)
		}
	}

	s.SetReady(false)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz while draining: got %d", rec.Code)
	}
}

func TestMetricsAndNotFound(t *testing.T) {
	h := newTestServer(t, config.Config{}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "satark_gateway_http_requests_total") {
		t.Fatal("metrics missing satark_gateway_http_requests_total")
	}
}
