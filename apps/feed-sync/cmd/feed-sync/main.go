// Command feed-sync downloads public threat feeds (URLhaus, OpenPhish) and
// stores them for url-intel. It runs to completion and is scheduled as a
// Kubernetes CronJob.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/feeds"
	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/store"
	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/syncer"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	timeout := envDuration("FEED_SYNC_TIMEOUT", 5*time.Minute)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sinks, closeSinks, err := buildSinks(ctx, log)
	if err != nil {
		log.Error("sink setup failed", "err", err)
		os.Exit(1)
	}
	defer closeSinks()

	s := &syncer.Syncer{
		Fetcher: feeds.NewFetcher(envDuration("FEED_SYNC_HTTP_TIMEOUT", 60*time.Second)),
		Feeds:   feeds.Defaults,
		Sinks:   sinks,
		Log:     log,
	}
	res := s.Run(ctx)
	if res.Failed == len(s.Feeds) {
		log.Error("all feeds failed")
		os.Exit(1)
	}
	if res.Failed > 0 {
		// Partial failure: keep the job green so the other feeds' data is
		// used, but the per-feed last-success metric/key will go stale.
		log.Warn("some feeds failed", "failed", res.Failed)
	}
}

// buildSinks connects to Valkey (FEED_SYNC_VALKEY_ADDR) and Postgres
// (FEED_SYNC_DATABASE_URL). With neither configured it falls back to a
// log-only dry run.
func buildSinks(ctx context.Context, log *slog.Logger) ([]syncer.Sink, func(), error) {
	var sinks []syncer.Sink
	var closers []func()
	closeAll := func() {
		for _, c := range closers {
			c()
		}
	}
	if addr := os.Getenv("FEED_SYNC_VALKEY_ADDR"); addr != "" {
		c := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("FEED_SYNC_VALKEY_PASSWORD")})
		closers = append(closers, func() { _ = c.Close() })
		if err := c.Ping(ctx).Err(); err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("valkey: %w", err)
		}
		sinks = append(sinks, &store.Valkey{C: c})
	}
	if dsn := os.Getenv("FEED_SYNC_DATABASE_URL"); dsn != "" {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("postgres: %w", err)
		}
		closers = append(closers, pool.Close)
		pg := &store.Postgres{Pool: pool}
		if err := pg.Migrate(ctx); err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("postgres migrate: %w", err)
		}
		sinks = append(sinks, pg)
	}
	if len(sinks) == 0 {
		log.Warn("no sinks configured; dry run")
		sinks = append(sinks, syncer.LogSink{Log: log})
	}
	return sinks, closeAll, nil
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return d
	}
	return def
}
