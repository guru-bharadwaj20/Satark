package urlheur

import (
	"context"
	"net/url"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

func target(raw string, chain ...string) *analyzer.Target {
	u, _ := url.Parse(raw)
	return &analyzer.Target{URL: u, Host: u.Hostname(), RedirectChain: chain}
}

func ids(f analyzer.Finding) map[string]bool {
	m := map[string]bool{}
	for _, s := range f.Signals {
		m[s.ID] = true
	}
	return m
}

func TestHeuristics(t *testing.T) {
	c := Check{}
	if got := ids(c.Run(context.Background(), target("http://203.0.113.9/pay"))); !got["url.raw_ip"] {
		t.Fatal("raw ipv4")
	}
	if got := ids(c.Run(context.Background(), target("http://[2606:4700::1]/"))); !got["url.raw_ip"] {
		t.Fatal("raw ipv6")
	}
	for _, raw := range []string{
		"https://files.example/SBI_KYC_Update.APK",
		"https://cdn.example/dl.php?file=bill-pay.apk",
		"https://cdn.example/app.xapk",
	} {
		if !ids(c.Run(context.Background(), target(raw)))["url.apk_download"] {
			t.Errorf("%s: apk not detected", raw)
		}
	}
	// APK reached through a redirect from an innocent-looking link.
	tg := target("https://cdn.example/x.apk", "https://bit.ly/abc", "https://cdn.example/x.apk")
	if !ids(c.Run(context.Background(), tg))["url.apk_download"] {
		t.Fatal("apk in chain")
	}
	if f := c.Run(context.Background(), target("https://www.example.com/apk-guide.html")); len(f.Signals) != 0 {
		t.Fatalf("false positive %+v", f.Signals)
	}
}
