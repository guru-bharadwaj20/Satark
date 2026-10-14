// Package syncer runs one sync pass: fetch every feed and hand the entries
// to each configured sink.
package syncer

import (
	"context"
	"log/slog"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/feeds"
)

// Sink stores a feed snapshot. Store must replace the previous snapshot of
// that feed atomically, so readers never see a half-loaded feed.
type Sink interface {
	Name() string
	Store(ctx context.Context, feed string, entries []feeds.Entry, at time.Time) error
}

// LogSink only logs; it is useful for dry runs.
type LogSink struct{ Log *slog.Logger }

// Name implements Sink.
func (LogSink) Name() string { return "log" }

// Store implements Sink.
func (s LogSink) Store(_ context.Context, feed string, entries []feeds.Entry, _ time.Time) error {
	s.Log.Info("feed snapshot", "feed", feed, "entries", len(entries))
	return nil
}

// Fetcher abstracts feeds.Fetcher for tests.
type Fetcher interface {
	Fetch(ctx context.Context, f feeds.Feed) ([]feeds.Entry, error)
}

// Syncer wires feeds to sinks.
type Syncer struct {
	Fetcher Fetcher
	Feeds   []feeds.Feed
	Sinks   []Sink
	Log     *slog.Logger
	Now     func() time.Time
}

// Result summarises a run.
type Result struct {
	Entries map[string]int
	Failed  int
}

// Run syncs every feed. A feed counts as failed if fetching it or storing it
// in any sink fails.
func (s *Syncer) Run(ctx context.Context) Result {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	res := Result{Entries: map[string]int{}}
	for _, f := range s.Feeds {
		start := now()
		entries, err := s.Fetcher.Fetch(ctx, f)
		if err != nil {
			s.Log.Error("fetch failed", "feed", f.Name, "err", err)
			res.Failed++
			continue
		}
		ok := true
		for _, sink := range s.Sinks {
			if err := sink.Store(ctx, f.Name, entries, start); err != nil {
				s.Log.Error("store failed", "feed", f.Name, "sink", sink.Name(), "err", err)
				ok = false
			}
		}
		if !ok {
			res.Failed++
			continue
		}
		res.Entries[f.Name] = len(entries)
		s.Log.Info("feed synced", "feed", f.Name, "entries", len(entries), "duration_ms", now().Sub(start).Milliseconds())
	}
	return res
}
