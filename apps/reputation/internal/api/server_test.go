package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/ratelimit"
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
	if rec.Code != 200 || out["status"] != "insufficient_reports" || out["reporters"] != nil {
		t.Fatalf("below threshold must reveal nothing: %d %v", rec.Code, out)
	}
	report(t, s, "tg:3", "9000000001")
	_, out = do(t, s, "GET", lookupPath("phone", "9000000001"), "")
	if out["status"] != "reported" || out["reporters"] != "fewer than 10" || out["subject"] != "90XXXXXX01" {
		t.Fatalf("lookup: %v", out)
	}
	if cats := out["categories"].([]any); len(cats) != 1 || cats[0] != "kyc_fraud" {
		t.Fatalf("categories: %v", out["categories"])
	}
	if strings.Contains(strings.ToLower(out["message"].(string)), "scammer") {
		t.Fatal("message must not label anyone")
	}
}

func TestRateLimit(t *testing.T) {
	s := New(Config{
		Store:   store.NewMemory(),
		Hasher:  identifier.NewHasher("test"),
		Limiter: ratelimit.New(nil, ratelimit.Window{Max: 2, Period: time.Hour}),
	})
	report(t, s, "tg:9", "9000000001")
	report(t, s, "tg:9", "9000000002")
	rec := report(t, s, "tg:9", "9000000003")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("want 429 with Retry-After, got %d", rec.Code)
	}
	if rec := report(t, s, "tg:10", "9000000003"); rec.Code != http.StatusAccepted {
		t.Fatalf("other reporter limited: %d", rec.Code)
	}
}

func TestForget(t *testing.T) {
	s := newTestServer()
	report(t, s, "tg:1", "9000000001")
	report(t, s, "tg:1", "9000000002")
	report(t, s, "tg:2", "9000000001")
	_, out := do(t, s, "POST", "/v1/forget", `{"reporter_id":"tg:1"}`)
	if out["reports_deleted"].(float64) != 2 {
		t.Fatalf("forget: %v", out)
	}
	// tg:1 can report again (nothing left), tg:2's report is untouched.
	_, out = do(t, s, "POST", "/v1/reports", `{"kind":"phone","value":"9000000001","reporter_id":"tg:2"}`)
	if out["status"] != "duplicate" {
		t.Fatalf("other reporter affected: %v", out)
	}
	if rec, _ := do(t, s, "POST", "/v1/forget", `{}`); rec.Code != 400 {
		t.Fatalf("empty forget: %d", rec.Code)
	}
}

func TestBucket(t *testing.T) {
	for n, want := range map[int]string{3: "fewer than 10", 10: "10-49", 75: "50-99", 500: "100+"} {
		if got := Bucket(n); got != want {
			t.Errorf("Bucket(%d)=%q want %q", n, got, want)
		}
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
