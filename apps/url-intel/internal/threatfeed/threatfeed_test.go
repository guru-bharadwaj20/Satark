package threatfeed

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

func setup(t *testing.T) (*Store, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)
	mr.SAdd("satark:feed:openphish:urls", "http://phish.example/login")
	mr.SAdd("satark:feed:urlhaus:hosts", "malware-dl.example")
	mr.HSet(MetaKey, "openphish:last_success", "1790000000", "openphish:count", "1234")
	return &Store{C: redis.NewClient(&redis.Options{Addr: mr.Addr()}), Feeds: DefaultFeeds}, mr
}

func TestCheck(t *testing.T) {
	s, _ := setup(t)
	c := &Check{S: s}
	f := c.Run(context.Background(), &analyzer.Target{RedirectChain: []string{"https://bit.ly/x", "http://phish.example/login"}})
	if len(f.Signals) != 1 || f.Signals[0].ID != "feed.url_listed" {
		t.Fatalf("%+v %s", f.Signals, f.Error)
	}
	f = c.Run(context.Background(), &analyzer.Target{FinalURL: "http://malware-dl.example/other.apk"})
	if len(f.Signals) != 1 || f.Signals[0].ID != "feed.host_listed" {
		t.Fatalf("%+v", f.Signals)
	}
	if f := c.Run(context.Background(), &analyzer.Target{FinalURL: "https://ok.example/"}); len(f.Signals) != 0 {
		t.Fatalf("%+v", f.Signals)
	}
}

func TestCheckValkeyDown(t *testing.T) {
	s, mr := setup(t)
	mr.Close()
	f := (&Check{S: s}).Run(context.Background(), &analyzer.Target{FinalURL: "https://ok.example/"})
	if f.Error == "" {
		t.Fatal("expected error")
	}
}

func TestCollector(t *testing.T) {
	s, _ := setup(t)
	want := `
# HELP satark_threat_feed_last_success_timestamp_seconds Unix time of the last successful feed-sync for the feed.
# TYPE satark_threat_feed_last_success_timestamp_seconds gauge
satark_threat_feed_last_success_timestamp_seconds{feed="openphish"} 1.79e+09
`
	if err := testutil.CollectAndCompare(NewCollector(s), strings.NewReader(want), "satark_threat_feed_last_success_timestamp_seconds"); err != nil {
		t.Fatal(err)
	}
}
