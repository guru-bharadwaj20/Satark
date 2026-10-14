package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(b), &m); err != nil {
		t.Fatalf("not JSON: %q: %v", b, err)
	}
	return m
}

func TestLoggerFieldsAndTrace(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(LogOptions{Service: "url-intel", Version: "1.2.3", Env: "dev", Output: &buf})
	l.InfoContext(WithTraceID(context.Background(), "abc123"), "lookup done", "domain", "example.in")

	m := decode(t, buf.Bytes())
	for k, want := range map[string]string{"service": "url-intel", "version": "1.2.3", "env": "dev", "trace_id": "abc123", "level": "INFO", "msg": "lookup done"} {
		if m[k] != want {
			t.Errorf("%s = %v, want %v", k, m[k], want)
		}
	}
}

func TestLoggerRedactsPII(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(LogOptions{Service: "s", Output: &buf})
	l.Info("got message", "text", "win a prize", "note", "call +91 98765 43210 or pay xxxx@okaxis")

	m := decode(t, buf.Bytes())
	if m["text"] != redacted {
		t.Errorf("text not redacted: %v", m["text"])
	}
	if m["note"] != "call [PHONE] or pay [UPI]" {
		t.Errorf("note not masked: %v", m["note"])
	}
}

func TestAccessLogPropagatesRequestID(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(LogOptions{Service: "gateway-api", Output: &buf})
	var seen string
	h := AccessLog(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = TraceID(r.Context())
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/scan/url?u=secret", nil)
	req.Header.Set(TraceHeader, "req-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen != "req-1" || rec.Header().Get(TraceHeader) != "req-1" {
		t.Fatalf("request id not propagated: ctx=%q header=%q", seen, rec.Header().Get(TraceHeader))
	}
	m := decode(t, buf.Bytes())
	if m["path"] != "/v1/scan/url" || m["status"] != float64(418) || m["trace_id"] != "req-1" {
		t.Fatalf("bad access log: %v", m)
	}
}
