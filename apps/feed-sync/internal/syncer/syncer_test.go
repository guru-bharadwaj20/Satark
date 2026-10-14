package syncer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/feeds"
)

type fakeFetcher map[string][]feeds.Entry

func (f fakeFetcher) Fetch(_ context.Context, feed feeds.Feed) ([]feeds.Entry, error) {
	if e, ok := f[feed.Name]; ok {
		return e, nil
	}
	return nil, errors.New("down")
}

type memSink struct{ got map[string]int }

func (memSink) Name() string { return "mem" }
func (m memSink) Store(_ context.Context, feed string, e []feeds.Entry, _ time.Time) error {
	m.got[feed] = len(e)
	return nil
}

func TestRun(t *testing.T) {
	sink := memSink{got: map[string]int{}}
	s := &Syncer{
		Fetcher: fakeFetcher{"urlhaus": {{URL: "http://a.example/", Host: "a.example"}}},
		Feeds:   []feeds.Feed{{Name: "urlhaus"}, {Name: "openphish"}},
		Sinks:   []Sink{sink},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	res := s.Run(context.Background())
	if res.Failed != 1 || res.Entries["urlhaus"] != 1 || sink.got["urlhaus"] != 1 {
		t.Fatalf("%+v %+v", res, sink.got)
	}
	if _, ok := sink.got["openphish"]; ok {
		t.Fatal("failed feed must not be stored")
	}
}
