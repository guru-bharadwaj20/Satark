package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

func newTestServer() *Server {
	return New(Config{Store: store.NewMemory(), Hasher: identifier.NewHasher("test")})
}

func do(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func report(t *testing.T, h http.Handler, reporter, value string) *httptest.ResponseRecorder {
	t.Helper()
	rec, _ := do(t, h, "POST", "/v1/reports",
		`{"kind":"phone","value":"`+value+`","category":"kyc_fraud","reporter_id":"`+reporter+`"}`)
	return rec
}

func lookupPath(kind, value string) string {
	return "/v1/lookup?kind=" + kind + "&value=" + url.QueryEscape(value)
}

func TestReportAndLookup(t *testing.T) {
	s := newTestServer()
	if rec := report(t, s, "tg:1", "+91 90000 00001"); rec.Code != http.StatusAccepted {
		t.Fatalf("report: %d %s", rec.Code, rec.Body)
	}
	// Same reporter again, different formatting: duplicate.
	_, out := do(t, s, "POST", "/v1/reports", `{"kind":"phone","value":"09000000001","reporter_id":"tg:1"}`)
	if out["status"] != "duplicate" {
		t.Fatalf("want duplicate, got %v", out)
	}
	report(t, s, "tg:2", "9000000001")
	rec, out := do(t, s, "GET", lookupPath("phone", "9000000001"), "")
	if rec.Code != 200 || out["distinct_reporters"].(float64) != 2 {
		t.Fatalf("lookup: %d %v", rec.Code, out)
	}
}

func TestValidation(t *testing.T) {
	s := newTestServer()
	for _, body := range []string{
		`not json`,
		`{"kind":"email","value":"a@b.c","reporter_id":"x"}`,
		`{"kind":"phone","value":"123","reporter_id":"x"}`,
		`{"kind":"phone","value":"9000000001","category":"nope","reporter_id":"x"}`,
		`{"kind":"phone","value":"9000000001"}`,
	} {
		if rec, _ := do(t, s, "POST", "/v1/reports", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d", body, rec.Code)
		}
	}
}

func TestHealth(t *testing.T) {
	s := newTestServer()
	for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
		if rec, _ := do(t, s, "GET", p, ""); rec.Code != 200 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}
