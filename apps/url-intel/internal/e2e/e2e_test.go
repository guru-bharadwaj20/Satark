// Package e2e runs the full url-intel pipeline against recorded network
// fixtures (testdata/index.json): redirects, RDAP, crt.sh and Safe Browsing.
// DNS and Valkey are faked in-process.
package e2e

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/allowlist"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/ctlog"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/dnscheck"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainage"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/expand"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/fixture"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/homoglyph"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/lookalike"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/normalize"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/safebrowsing"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/threatfeed"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/urlheur"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type resolver struct{}

func (resolver) LookupIPAddr(_ context.Context, h string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("151.101.0.1")}}, nil
}
func (resolver) LookupNS(_ context.Context, h string) ([]*net.NS, error) {
	return []*net.NS{{Host: "ns1." + h + "."}}, nil
}
func (resolver) LookupMX(_ context.Context, h string) ([]*net.MX, error) {
	if h == "hdfcbank-kyc-update.example" {
		return nil, &net.DNSError{Err: "no such host", Name: h, IsNotFound: true}
	}
	return []*net.MX{{Host: "mx." + h + "."}}, nil
}

func pipeline(t *testing.T) (*analyzer.Analyzer, *fixture.Replay, *miniredis.Miniredis) {
	t.Helper()
	rp, err := fixture.Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	client := rp.Client()

	ex := expand.New(time.Second, 5)
	ex.Client = client

	da := domainage.New(client)
	da.Now = func() time.Time { return now }
	da.Dial = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("unexpected WHOIS fallback")
		return nil, nil
	}

	ct := ctlog.New(client)
	ct.Now = func() time.Time { return now }

	sb := safebrowsing.New(client, "test-key", "test")

	mr := miniredis.RunT(t)
	feeds := &threatfeed.Store{C: redis.NewClient(&redis.Options{Addr: mr.Addr()}), Feeds: threatfeed.DefaultFeeds}

	brands := lookalike.Default()
	a := analyzer.New(
		analyzer.WithPreparers(normalize.Preparer{}, expand.Preparer{E: ex}),
		analyzer.WithAllowlist(allowlist.Default(brands.OfficialDomains()...)),
		analyzer.WithCheckTimeout(2*time.Second),
		analyzer.WithChecks(
			&dnscheck.Check{R: resolver{}},
			&domainage.Check{L: da},
			ct,
			&lookalike.Check{D: brands},
			&homoglyph.Check{Brands: brands},
			urlheur.Check{},
			&safebrowsing.Check{C: sb},
			&threatfeed.Check{S: feeds},
		),
	)
	return a, rp, mr
}

func signalIDs(rep *analyzer.Report) map[string]bool {
	m := map[string]bool{}
	for _, s := range rep.Signals {
		m[s.ID] = true
	}
	return m
}

func TestPhishingShortLink(t *testing.T) {
	a, _, mr := pipeline(t)
	mr.SAdd("satark:feed:openphish:urls", "https://hdfcbank-kyc-update.example/verify?ref=sms")

	rep, err := a.Analyze(context.Background(), "https://bit.ly/3kycUPD")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Target.FinalURL != "https://hdfcbank-kyc-update.example/verify?ref=sms" {
		t.Fatalf("final url %s", rep.Target.FinalURL)
	}
	got := signalIDs(rep)
	for _, want := range []string{
		"redirect.shortener", "domain.very_new", "ct.first_seen_recent",
		"lookalike.brand_in_domain", "dns.no_mx", "feed.url_listed",
	} {
		if !got[want] {
			t.Errorf("missing signal %s; got %v", want, got)
		}
	}
	if rep.Score < 90 {
		t.Errorf("score %d too low for a listed phishing link", rep.Score)
	}
	for _, f := range rep.Findings {
		if f.Error != "" {
			t.Errorf("check %s errored: %s", f.Check, f.Error)
		}
	}
}

func TestEstablishedSite(t *testing.T) {
	a, _, _ := pipeline(t)
	rep, err := a.Analyze(context.Background(), "https://www.wikipedia.org/")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Score != 0 {
		t.Fatalf("score %d, signals %+v", rep.Score, rep.Signals)
	}
}

func TestOfficialSiteSkipsHeuristics(t *testing.T) {
	a, rp, _ := pipeline(t)
	rep, err := a.Analyze(context.Background(), "https://uidai.gov.in/")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Allowlisted == "" || rep.Score != 0 {
		t.Fatalf("%+v", rep)
	}
	for _, s := range rp.Seen {
		if s == "GET https://rdap.org/domain/uidai.gov.in" {
			t.Fatal("allowlisted host must not trigger RDAP")
		}
	}
}
