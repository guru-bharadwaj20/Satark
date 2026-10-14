package cache

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

type counting struct {
	n     atomic.Int32
	score int
	err   string
}

func (c *counting) Analyze(_ context.Context, raw string) (*analyzer.Report, error) {
	c.n.Add(1)
	rep := &analyzer.Report{Score: c.score, Target: analyzer.Target{Raw: raw}}
	if c.err != "" {
		rep.Findings = []analyzer.Finding{{Check: "x", Error: c.err}}
	}
	return rep, nil
}

func newCache(t *testing.T, inner Inner) (*Analyzer, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return New(inner, c, 30*time.Minute, 6*time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)), nil), mr
}

func TestCacheHitAndTTL(t *testing.T) {
	inner := &counting{score: 10}
	a, mr := newCache(t, inner)
	ctx := context.Background()
	for _, raw := range []string{"https://Example.com/a", "HTTPS://example.com:443/a", " https://example.com/a#x"} {
		if _, err := a.Analyze(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}
	if n := inner.n.Load(); n != 1 {
		t.Fatalf("inner called %d times; equivalent URLs must share one entry", n)
	}
	key := Key("https://example.com/a")
	if ttl := mr.TTL(key); ttl != 30*time.Minute {
		t.Fatalf("ttl %v", ttl)
	}
	mr.FastForward(31 * time.Minute)
	if _, err := a.Analyze(ctx, "https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	if inner.n.Load() != 2 {
		t.Fatal("expired entry should be recomputed")
	}
}

func TestRiskyAndPartialTTL(t *testing.T) {
	a, mr := newCache(t, &counting{score: 80})
	a.Analyze(context.Background(), "https://bad.example/")
	if ttl := mr.TTL(Key("https://bad.example/")); ttl != 6*time.Hour {
		t.Fatalf("risky ttl %v", ttl)
	}
	b, mr2 := newCache(t, &counting{score: 80, err: "timeout"})
	b.Analyze(context.Background(), "https://bad.example/")
	if ttl := mr2.TTL(Key("https://bad.example/")); ttl != 5*time.Minute {
		t.Fatalf("partial ttl %v", ttl)
	}
}

func TestCacheDownStillAnalyzes(t *testing.T) {
	inner := &counting{}
	a, mr := newCache(t, inner)
	mr.Close()
	if _, err := a.Analyze(context.Background(), "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	if inner.n.Load() != 1 {
		t.Fatal("inner not called")
	}
}
