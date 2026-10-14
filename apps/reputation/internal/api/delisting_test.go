package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

func TestDelistingFlow(t *testing.T) {
	s := New(Config{Store: store.NewMemory(), Hasher: identifier.NewHasher("t"), AdminToken: "secret"})
	for _, r := range []string{"tg:1", "tg:2", "tg:3"} {
		report(t, s, r, "9000000001")
	}
	rec, out := do(t, s, "POST", "/v1/delisting-requests",
		`{"kind":"phone","value":"9000000001","reason":"This is my shop's number, not a fraud line.","contact":"owner@example.com"}`)
	if rec.Code != http.StatusAccepted || out["status"] != "pending" {
		t.Fatalf("create: %d %v", rec.Code, out)
	}

	// Admin endpoints require the token.
	if rec, _ := do(t, s, "GET", "/v1/admin/delisting-requests", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: %d", rec.Code)
	}
	adm := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		return rr
	}
	if rr := adm("GET", "/v1/admin/delisting-requests?status=pending", ""); !strings.Contains(rr.Body.String(), "90XXXXXX01") {
		t.Fatalf("list should show masked subject: %s", rr.Body)
	}
	if rr := adm("POST", "/v1/admin/delisting-requests/1", `{"decision":"approve","note":"verified GST"}`); rr.Code != 200 ||
		!strings.Contains(rr.Body.String(), `"reports_removed":3`) {
		t.Fatalf("approve: %d %s", rr.Code, rr.Body)
	}
	if rr := adm("POST", "/v1/admin/delisting-requests/1", `{"decision":"reject"}`); rr.Code != http.StatusNotFound {
		t.Fatalf("double decision: %d", rr.Code)
	}
	_, out = do(t, s, "GET", lookupPath("phone", "9000000001"), "")
	if out["status"] != "insufficient_reports" {
		t.Fatalf("after delisting: %v", out)
	}
}

func TestDelistingValidation(t *testing.T) {
	s := newTestServer()
	if rec, _ := do(t, s, "POST", "/v1/delisting-requests", `{"kind":"phone","value":"9000000001","reason":"short"}`); rec.Code != 400 {
		t.Fatalf("short reason: %d", rec.Code)
	}
	if rec, _ := do(t, s, "GET", "/v1/admin/delisting-requests", ""); rec.Code != 401 {
		t.Fatalf("admin disabled without token: %d", rec.Code)
	}
}
