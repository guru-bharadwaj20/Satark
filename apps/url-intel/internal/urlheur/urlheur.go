// Package urlheur holds URL-shape heuristics that need no network access:
// links to raw IP addresses and links that download Android APK files.
// Fake "bank KYC" and "electricity bill" APKs are a leading cause of
// account takeover in India.
package urlheur

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainutil"
)

var apkExtensions = []string{".apk", ".xapk", ".apks", ".apkm"}

// IsAPK reports whether u points at an Android package, either by its path
// or by a file name carried in the query string (download.php?file=x.apk).
func IsAPK(u *url.URL) bool {
	if hasAPKExt(path.Base(u.Path)) {
		return true
	}
	for _, vs := range u.Query() {
		for _, v := range vs {
			if hasAPKExt(path.Base(v)) {
				return true
			}
		}
	}
	return false
}

func hasAPKExt(name string) bool {
	name = strings.ToLower(name)
	for _, e := range apkExtensions {
		if strings.HasSuffix(name, e) {
			return true
		}
	}
	return false
}

// Check implements analyzer.Check.
type Check struct{}

// Name implements analyzer.Check.
func (Check) Name() string { return "url_heuristics" }

// Run implements analyzer.Check.
func (Check) Run(_ context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	if domainutil.IsIP(t.Host) {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "url.raw_ip", Severity: analyzer.SeverityHigh, Weight: 25,
			Detail: "link points to a bare IP address instead of a named website"})
	}
	urls := t.RedirectChain
	if len(urls) == 0 && t.URL != nil {
		urls = []string{t.URL.String()}
	}
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if IsAPK(u) {
			f.Signals = append(f.Signals, analyzer.Signal{ID: "url.apk_download", Severity: analyzer.SeverityHigh, Weight: 40,
				Detail: "link downloads an Android app (APK); never install apps from links in messages"})
			break
		}
	}
	return f
}
