// Package threatfeed checks URLs against the threat-feed sets that
// feed-sync loads into Valkey. Key contract (see apps/feed-sync/internal/store):
//
//	satark:feed:<feed>:urls   SET of canonical URLs
//	satark:feed:<feed>:hosts  SET of hostnames
//	satark:feed:meta          HASH <feed>:last_success, <feed>:count
package threatfeed

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

// MetaKey is the feed metadata hash.
const MetaKey = "satark:feed:meta"

// DefaultFeeds are the feeds synced by feed-sync.
var DefaultFeeds = []string{"urlhaus", "openphish"}

// Match is one feed hit.
type Match struct {
	Feed string `json:"feed"`
	Kind string `json:"kind"` // url | host
	URL  string `json:"url"`
}

// Store looks up feed membership in Valkey.
type Store struct {
	C     redis.UniversalClient
	Feeds []string
}

// Lookup checks every URL (and its host) against every feed in one round trip.
func (s *Store) Lookup(ctx context.Context, urls []string) ([]Match, error) {
	type q struct {
		m   Match
		cmd *redis.BoolCmd
	}
	var qs []q
	pipe := s.C.Pipeline()
	hosts := map[string]bool{}
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		for _, f := range s.Feeds {
			qs = append(qs, q{Match{Feed: f, Kind: "url", URL: raw}, pipe.SIsMember(ctx, "satark:feed:"+f+":urls", raw)})
			if host != "" && !hosts[f+"|"+host] {
				hosts[f+"|"+host] = true
				qs = append(qs, q{Match{Feed: f, Kind: "host", URL: raw}, pipe.SIsMember(ctx, "satark:feed:"+f+":hosts", host)})
			}
		}
	}
	if len(qs) == 0 {
		return nil, nil
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	var out []Match
	for _, x := range qs {
		if x.cmd.Val() {
			out = append(out, x.m)
		}
	}
	return out, nil
}

// Check implements analyzer.ThreatCheck.
type Check struct{ S *Store }

// Name implements analyzer.Check.
func (*Check) Name() string { return "threat_feeds" }

// ThreatIntel marks the check as running for allowlisted hosts too.
func (*Check) ThreatIntel() {}

// Run implements analyzer.Check.
func (c *Check) Run(ctx context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	urls := t.RedirectChain
	if len(urls) == 0 {
		urls = []string{t.FinalURL}
	}
	matches, err := c.S.Lookup(ctx, urls)
	if err != nil {
		f.Error = err.Error()
		return f
	}
	if len(matches) == 0 {
		return f
	}
	f.Data = matches
	urlHit, hostHit := "", ""
	for _, m := range matches {
		if m.Kind == "url" && urlHit == "" {
			urlHit = m.Feed
		}
		if m.Kind == "host" && hostHit == "" {
			hostHit = m.Feed
		}
	}
	if urlHit != "" {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "feed.url_listed", Severity: analyzer.SeverityHigh, Weight: 70,
			Detail: "this exact link is listed as malicious by " + urlHit})
	} else if hostHit != "" {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "feed.host_listed", Severity: analyzer.SeverityHigh, Weight: 35,
			Detail: "this website has hosted malicious links reported to " + hostHit})
	}
	return f
}

// Collector exports feed freshness from Valkey as Prometheus metrics, so
// staleness can be alerted on even though feed-sync is a short-lived job.
type Collector struct {
	S       *Store
	last    *prometheus.Desc
	entries *prometheus.Desc
}

// NewCollector returns a Collector for s.
func NewCollector(s *Store) *Collector {
	return &Collector{
		S: s,
		last: prometheus.NewDesc("satark_threat_feed_last_success_timestamp_seconds",
			"Unix time of the last successful feed-sync for the feed.", []string{"feed"}, nil),
		entries: prometheus.NewDesc("satark_threat_feed_entries",
			"Entries in the current snapshot of the feed.", []string{"feed"}, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.last
	ch <- c.entries
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	vals, err := c.S.C.HGetAll(context.Background(), MetaKey).Result()
	if err != nil {
		return
	}
	for _, f := range c.S.Feeds {
		if v, err := strconv.ParseFloat(vals[f+":last_success"], 64); err == nil {
			ch <- prometheus.MustNewConstMetric(c.last, prometheus.GaugeValue, v, f)
		}
		if v, err := strconv.ParseFloat(vals[f+":count"], 64); err == nil {
			ch <- prometheus.MustNewConstMetric(c.entries, prometheus.GaugeValue, v, f)
		}
	}
}
