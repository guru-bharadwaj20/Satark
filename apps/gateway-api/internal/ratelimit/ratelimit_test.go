package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setup(t *testing.T) (*miniredis.Miniredis, *Limiter) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, New(rdb, Options{Timeout: time.Second})
}

func TestBurstThenDeny(t *testing.T) {
	mr, l := setup(t)
	mr.SetTime(time.Unix(1_700_000_000, 0))
	ctx := context.Background()
	lim := Limit{Rate: 1, Burst: 3}
	for i := 0; i < 3; i++ {
		res, err := l.Allow(ctx, "c1", lim)
		if err != nil || !res.Allowed {
			t.Fatalf("request %d: %+v %v", i, res, err)
		}
		if res.Remaining != 2-i {
			t.Fatalf("remaining = %d", res.Remaining)
		}
	}
	res, err := l.Allow(ctx, "c1", lim)
	if err != nil || res.Allowed {
		t.Fatalf("4th request should be denied: %+v %v", res, err)
	}
	if res.RetryAfter != time.Second {
		t.Fatalf("retry after = %v", res.RetryAfter)
	}
	// Other clients have their own bucket.
	if res, _ := l.Allow(ctx, "c2", lim); !res.Allowed {
		t.Fatal("c2 should be allowed")
	}
}

func TestRefill(t *testing.T) {
	mr, l := setup(t)
	base := time.Unix(1_700_000_000, 0)
	mr.SetTime(base)
	ctx := context.Background()
	lim := Limit{Rate: 2, Burst: 2}
	for i := 0; i < 2; i++ {
		_, _ = l.Allow(ctx, "c", lim)
	}
	if res, _ := l.Allow(ctx, "c", lim); res.Allowed {
		t.Fatal("bucket should be empty")
	}
	mr.SetTime(base.Add(500 * time.Millisecond)) // +1 token at 2/s
	if res, _ := l.Allow(ctx, "c", lim); !res.Allowed {
		t.Fatal("one token should have refilled")
	}
	mr.SetTime(base.Add(10 * time.Second)) // capped at burst
	if res, _ := l.Allow(ctx, "c", lim); !res.Allowed || res.Remaining != 1 {
		t.Fatalf("refill must cap at burst: %+v", res)
	}
}

func TestBucketExpires(t *testing.T) {
	mr, l := setup(t)
	_, _ = l.Allow(context.Background(), "idle", Limit{Rate: 1, Burst: 5})
	ttl := mr.TTL(keyPrefix + "idle")
	if ttl <= 0 || ttl > 7*time.Second {
		t.Fatalf("ttl = %v", ttl)
	}
}

func TestInvalidLimit(t *testing.T) {
	_, l := setup(t)
	if _, err := l.Allow(context.Background(), "x", Limit{}); err == nil {
		t.Fatal("want error")
	}
	res, err := l.AllowN(context.Background(), "x", Limit{Rate: 1, Burst: 2}, 3)
	if err != nil || res.Allowed {
		t.Fatal("cost above burst can never succeed")
	}
}

func TestClientKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	if got := ClientKey(r); got != "ip:10.0.0.5" {
		t.Fatalf("got %q", got)
	}
	// Envoy appends the peer address; earlier entries are client-supplied.
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := ClientKey(r); got != "ip:203.0.113.9" {
		t.Fatalf("got %q", got)
	}
	r.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.9")
	if got := ClientKey(r); got != "ip:203.0.113.9" {
		t.Fatalf("forged first entry must be ignored, got %q", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.9, garbage")
	if got := ClientKey(r); got != "ip:10.0.0.5" {
		t.Fatalf("unparseable last entry falls back to the peer, got %q", got)
	}
	// An unchecked API key must not buy a separate bucket.
	r.Header.Del("X-Forwarded-For")
	r.Header.Set("X-API-Key", "made-up-1")
	if got := ClientKey(r); got != "ip:10.0.0.5" {
		t.Fatalf("api key must not change the key, got %q", got)
	}
}

func TestMiddleware(t *testing.T) {
	mr, l := setup(t)
	mr.SetTime(time.Unix(1_700_000_000, 0))
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := l.Middleware(Limit{Rate: 1, Burst: 1}, ClientKey, ok)

	do := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/scan/text", nil)
		r.RemoteAddr = "192.0.2.1:5555"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := do(); w.Code != http.StatusOK || w.Header().Get("X-RateLimit-Limit") != "1" {
		t.Fatalf("first: %d %v", w.Code, w.Header())
	}
	w := do()
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("second: %d %v", w.Code, w.Header())
	}

	mr.Close() // fail open
	if w := do(); w.Code != http.StatusOK {
		t.Fatalf("should fail open, got %d", w.Code)
	}
}
