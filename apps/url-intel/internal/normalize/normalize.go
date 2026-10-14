// Package normalize canonicalises and validates user-submitted URLs.
//
// Users paste links from SMS and WhatsApp, so input is messy: missing
// schemes, "defanged" forms such as hxxp://example[.]com, trailing
// punctuation, upper-case hosts and Unicode domains. Normalize turns this
// into a single canonical URL that every downstream check agrees on.
package normalize

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"golang.org/x/net/idna"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/netguard"
)

// MaxLength is the longest URL accepted, in bytes.
const MaxLength = 2048

// Validation errors. They all wrap analyzer.ErrInvalidInput.
var (
	ErrEmpty        = fmt.Errorf("%w: empty url", analyzer.ErrInvalidInput)
	ErrTooLong      = fmt.Errorf("%w: url longer than %d bytes", analyzer.ErrInvalidInput, MaxLength)
	ErrScheme       = fmt.Errorf("%w: only http and https urls are supported", analyzer.ErrInvalidInput)
	ErrNoHost       = fmt.Errorf("%w: url has no host", analyzer.ErrInvalidInput)
	ErrBadHost      = fmt.Errorf("%w: host is not a valid domain name or ip address", analyzer.ErrInvalidInput)
	ErrPrivateHost  = fmt.Errorf("%w: private, loopback and reserved addresses are not analysed", analyzer.ErrInvalidInput)
	ErrControlChars = fmt.Errorf("%w: url contains control characters", analyzer.ErrInvalidInput)
)

var defang = strings.NewReplacer(
	"hxxps://", "https://", "hxxp://", "http://", "HXXPS://", "https://", "HXXP://", "http://",
	"[.]", ".", "(.)", ".", "{.}", ".", "[dot]", ".", "(dot)", ".", "[:]", ":",
)

var idnaProfile = idna.New(
	idna.MapForLookup(),
	idna.BidiRule(),
	idna.Transitional(false),
)

// Result is a normalised URL plus any signals noticed while normalising.
type Result struct {
	URL     *url.URL
	Signals []analyzer.Signal
}

// Normalize canonicalises raw. The returned URL has a lower-case scheme and
// ASCII (punycode) host, no default port, no fragment and no userinfo.
func Normalize(raw string) (*Result, error) {
	s := strings.TrimSpace(raw)
	for prev := ""; prev != s; {
		prev = s
		s = strings.Trim(s, "<>\"'`")
		s = strings.TrimRight(s, ".,;:!?)]}")
	}
	if s == "" {
		return nil, ErrEmpty
	}
	if len(s) > MaxLength {
		return nil, ErrTooLong
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return nil, ErrControlChars
		}
	}
	s = defang.Replace(s)

	lower := strings.ToLower(s)
	if !strings.Contains(lower, "://") {
		if strings.Contains(lower, ":") && !hostPortLike(lower) {
			return nil, ErrScheme
		}
		s = "http://" + strings.TrimPrefix(s, "//")
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", analyzer.ErrInvalidInput, err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrScheme
	}

	res := &Result{}
	if u.User != nil {
		res.Signals = append(res.Signals, analyzer.Signal{
			ID: "url.userinfo", Severity: analyzer.SeverityHigh, Weight: 30,
			Detail: "URL hides its real destination behind text before an @ sign",
		})
		u.User = nil
	}

	host, err := canonicalHost(u.Hostname())
	if err != nil {
		return nil, err
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		res.Signals = append(res.Signals, analyzer.Signal{
			ID: "url.nonstandard_port", Severity: analyzer.SeverityLow, Weight: 10,
			Detail: "URL uses non-standard port " + port,
		})
	}
	u.Host = joinHostPort(host, port)
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	res.URL = u
	return res, nil
}

func hostPortLike(s string) bool {
	// "example.com:8080/path" has a colon but no scheme.
	hostport, _, _ := strings.Cut(s, "/")
	_, port, ok := strings.Cut(hostport, ":")
	if !ok || port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func canonicalHost(h string) (string, error) {
	h = strings.TrimSuffix(strings.ToLower(h), ".")
	if h == "" {
		return "", ErrNoHost
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		if !netguard.IsPublic(ip) {
			return "", ErrPrivateHost
		}
		return ip.Unmap().String(), nil
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return "", ErrPrivateHost
	}
	ascii, err := idnaProfile.ToASCII(h)
	if err != nil {
		return "", errors.Join(ErrBadHost, err)
	}
	if !strings.Contains(ascii, ".") || len(ascii) > 253 {
		return "", ErrBadHost
	}
	return ascii, nil
}

func joinHostPort(host, port string) string {
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port == "" {
		return host
	}
	return host + ":" + port
}

// Preparer adapts Normalize to the analyzer pipeline.
type Preparer struct{}

// Prepare normalises t.Raw in place.
func (Preparer) Prepare(_ context.Context, t *analyzer.Target) ([]analyzer.Signal, error) {
	res, err := Normalize(t.Raw)
	if err != nil {
		return nil, err
	}
	t.Normalized = res.URL.String()
	t.FinalURL = t.Normalized
	t.URL = res.URL
	return res.Signals, nil
}
