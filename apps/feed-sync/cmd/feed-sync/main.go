// Command feed-sync downloads public threat feeds (URLhaus, OpenPhish) and
// stores them for url-intel. It runs to completion and is scheduled as a
// Kubernetes CronJob.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/feeds"
	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/syncer"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	timeout := envDuration("FEED_SYNC_TIMEOUT", 5*time.Minute)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s := &syncer.Syncer{
		Fetcher: feeds.NewFetcher(envDuration("FEED_SYNC_HTTP_TIMEOUT", 60*time.Second)),
		Feeds:   feeds.Defaults,
		Sinks:   []syncer.Sink{syncer.LogSink{Log: log}},
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

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return d
	}
	return def
}
