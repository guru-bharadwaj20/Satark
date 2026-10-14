package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMiddlewareRecordsRED(t *testing.T) {
	reg := NewRegistry()
	m := NewHTTPMetrics(reg, "gateway-api")

	ok := m.Middleware("/v1/scan/url", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	bad := m.Middleware("/v1/scan/url", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))

	for i := 0; i < 3; i++ {
		ok.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/scan/url", nil))
	}
	bad.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/scan/url", nil))

	if got := testutil.ToFloat64(m.requests.WithLabelValues("POST", "/v1/scan/url", "200")); got != 3 {
		t.Fatalf("200 count = %v, want 3", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("POST", "/v1/scan/url", "502")); got != 1 {
		t.Fatalf("502 count = %v, want 1", got)
	}
	if n := testutil.CollectAndCount(m.duration); n != 1 {
		t.Fatalf("histogram series = %d, want 1", n)
	}
	if got := testutil.ToFloat64(m.inFlight); got != 0 {
		t.Fatalf("in-flight = %v, want 0", got)
	}
}

func TestMiddlewareCountsPanicAs500(t *testing.T) {
	m := NewHTTPMetrics(NewRegistry(), "svc")
	h := m.Middleware("/boom", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("x") }))
	func() {
		defer func() { _ = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))
	}()
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/boom", "500")); got != 1 {
		t.Fatalf("500 count = %v, want 1", got)
	}
}

func TestHandlerExposesServiceLabel(t *testing.T) {
	m := NewHTTPMetrics(NewRegistry(), "url-intel")
	m.Observe("GET", "/healthz", 200, 0)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `satark_http_requests_total{code="200",method="GET",route="/healthz",service="url-intel"} 1`) {
		t.Fatalf("metrics output missing series:\n%s", body)
	}
}
