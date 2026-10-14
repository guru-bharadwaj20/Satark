package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/feeds"
)

var snapshot1 = []feeds.Entry{
	{URL: "http://phish.example/login", Host: "phish.example"},
	{URL: "https://raw.githubusercontent.com/x/y/main/a.apk", Host: "raw.githubusercontent.com"},
}

func TestValkeyStoreSwapsAtomically(t *testing.T) {
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	v := &Valkey{C: c}
	ctx := context.Background()
	at := time.Unix(1_790_000_000, 0)

	if err := v.Store(ctx, "openphish", snapshot1, at); err != nil {
		t.Fatal(err)
	}
	if ok, _ := c.SIsMember(ctx, URLsKey("openphish"), "http://phish.example/login").Result(); !ok {
		t.Fatal("url missing")
	}
	if ok, _ := c.SIsMember(ctx, HostsKey("openphish"), "raw.githubusercontent.com").Result(); ok {
		t.Fatal("shared host must not be stored")
	}
	if got := mr.HGet(MetaKey, "openphish:last_success"); got != "1790000000" {
		t.Fatalf("meta %q", got)
	}

	// Second snapshot replaces the first entirely.
	snap2 := []feeds.Entry{{URL: "http://new.example/", Host: "new.example"}}
	if err := v.Store(ctx, "openphish", snap2, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.SCard(ctx, URLsKey("openphish")).Result(); n != 1 {
		t.Fatalf("expected replacement, got %d members", n)
	}
	if ttl := mr.TTL(URLsKey("openphish")); ttl != 0 {
		t.Fatalf("live key must not expire, ttl %v", ttl)
	}
	for _, k := range mr.Keys() {
		if len(k) > 4 && contains(k, ":tmp:") {
			t.Fatalf("temporary key left behind: %s", k)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestPostgres runs against a real database when FEED_SYNC_TEST_DATABASE_URL
// is set (CI starts a postgres service container for this).
func TestPostgres(t *testing.T) {
	dsn := os.Getenv("FEED_SYNC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("FEED_SYNC_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	p := &Postgres{Pool: pool}
	if err := p.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM threat_feed_entries WHERE feed = 'test'`); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().UTC().Truncate(time.Second)
	if err := p.Store(ctx, "test", snapshot1, t0); err != nil {
		t.Fatal(err)
	}
	if err := p.Store(ctx, "test", snapshot1[:1], t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM threat_feed_entries WHERE feed='test' AND active`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active=%d", active)
	}
}
