package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setup(t *testing.T) (*miniredis.Miniredis, *Cache) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, New(rdb, Options{TTL: time.Hour, LongTTL: 24 * time.Hour})
}

func TestKey(t *testing.T) {
	k := Key("text", []byte{0xde, 0xad})
	if k != "satark:verdict:v1:text:dead" {
		t.Fatalf("key = %q", k)
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	mr, c := setup(t)
	ctx := context.Background()
	key := Key("text", []byte{1, 2, 3})
	if _, ok := c.Get(ctx, key); ok {
		t.Fatal("expected miss")
	}
	in := Verdict{RiskScore: 42, Verdict: "suspicious", Reasons: []string{"urgency"}}
	if err := c.Set(ctx, key, in); err != nil {
		t.Fatal(err)
	}
	out, ok := c.Get(ctx, key)
	if !ok || out.RiskScore != 42 || out.Verdict != "suspicious" || out.CachedAt.IsZero() {
		t.Fatalf("got %+v ok=%v", out, ok)
	}
	if ttl := mr.TTL(key); ttl != time.Hour {
		t.Fatalf("ttl = %v", ttl)
	}
}

func TestLikelyScamUsesLongTTL(t *testing.T) {
	mr, c := setup(t)
	key := Key("url", []byte{9})
	_ = c.Set(context.Background(), key, Verdict{RiskScore: 95, Verdict: "likely_scam"})
	if ttl := mr.TTL(key); ttl != 24*time.Hour {
		t.Fatalf("ttl = %v", ttl)
	}
	mr.FastForward(25 * time.Hour)
	if _, ok := c.Get(context.Background(), key); ok {
		t.Fatal("entry should have expired")
	}
}

func TestCorruptEntryIsDropped(t *testing.T) {
	mr, c := setup(t)
	key := Key("qr", []byte{7})
	_ = mr.Set(key, "{not json")
	if _, ok := c.Get(context.Background(), key); ok {
		t.Fatal("corrupt entry must be a miss")
	}
	if mr.Exists(key) {
		t.Fatal("corrupt entry must be deleted")
	}
}

func TestBackendDownIsMiss(t *testing.T) {
	mr, c := setup(t)
	mr.Close()
	key := Key("text", []byte{1})
	if _, ok := c.Get(context.Background(), key); ok {
		t.Fatal("expected miss when backend is down")
	}
	if err := c.Set(context.Background(), key, Verdict{}); err == nil {
		t.Fatal("expected error when backend is down")
	}
}

func TestInvalidate(t *testing.T) {
	mr, c := setup(t)
	key := Key("text", []byte{5})
	_ = c.Set(context.Background(), key, Verdict{Verdict: "no_red_flags"})
	if err := c.Invalidate(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if mr.Exists(key) || strings.Contains(strings.Join(mr.Keys(), ","), "text") {
		t.Fatal("key should be gone")
	}
}
