// Package expand follows HTTP redirects (short links such as bit.ly) up to a
// bounded number of hops, without downloading page bodies, so later checks
// can inspect where a link really lands.
package expand

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/netguard"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/normalize"
)

// DefaultMaxRedirects bounds how many hops are followed.
const DefaultMaxRedirects = 5

// userAgent identifies Satark politely; some shorteners block empty agents.
const userAgent = "Mozilla/5.0 (compatible; SatarkLinkCheck/1.0; +https://github.com/guru-bharadwaj20/satark)"

// Shorteners lists well-known URL shortener hosts.
var Shorteners = map[string]bool{
	"bit.ly": true, "bitly.com": true, "tinyurl.com": true, "t.co": true, "goo.gl": true,
	"ow.ly": true, "is.gd": true, "buff.ly": true, "cutt.ly": true, "rb.gy": true,
	"shorturl.at": true, "tiny.cc": true, "rebrand.ly": true, "t.ly": true, "s.id": true,
	"v.gd": true, "shorte.st": true, "adf.ly": true, "bl.ink": true, "short.io": true,
	"surl.li": true, "lnkd.in": true, "qr.ae": true, "u.to": true,
}

// ErrTooManyRedirects is reported when the hop limit is reached.
var ErrTooManyRedirects = errors.New("too many redirects")

// Expander follows redirects.
type Expander struct {
	Client       *http.Client
	MaxRedirects int
	// Validate canonicalises and vets every hop; defaults to normalize.Normalize.
	Validate func(raw string) (*url.URL, error)
}

// New returns an Expander whose client refuses non-public destinations.
func New(timeout time.Duration, maxRedirects int) *Expander {
	if maxRedirects <= 0 {
		maxRedirects = DefaultMaxRedirects
	}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           netguard.DialContext(&net.Dialer{Timeout: 3 * time.Second}),
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          50,
		IdleConnTimeout:       30 * time.Second,
		DisableCompression:    true,
	}
	return &Expander{
		Client: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		MaxRedirects: maxRedirects,
		Validate: func(raw string) (*url.URL, error) {
			r, err := normalize.Normalize(raw)
			if err != nil {
				return nil, err
			}
			return r.URL, nil
		},
	}
}

// Result describes an expansion.
type Result struct {
	Chain  []string // every URL visited, starting with the input
	Final  *url.URL
	Status int
	Err    error
}

// Expand follows redirects from start. Network errors are returned in
// Result.Err; the chain up to the failure is still populated.
func (e *Expander) Expand(ctx context.Context, start *url.URL) Result {
	res := Result{Chain: []string{start.String()}, Final: start}
	cur := start
	for hop := 0; ; hop++ {
		status, loc, err := e.step(ctx, cur)
		res.Status = status
		if err != nil {
			res.Err = err
			return res
		}
		if loc == "" {
			return res
		}
		if hop >= e.MaxRedirects {
			res.Err = ErrTooManyRedirects
			return res
		}
		ref, err := url.Parse(loc)
		if err != nil {
			res.Err = fmt.Errorf("bad Location header: %w", err)
			return res
		}
		next, err := e.Validate(cur.ResolveReference(ref).String())
		if err != nil {
			res.Err = fmt.Errorf("redirect target rejected: %w", err)
			return res
		}
		res.Chain = append(res.Chain, next.String())
		res.Final = next
		cur = next
	}
}

func (e *Expander) step(ctx context.Context, u *url.URL) (int, string, error) {
	status, loc, err := e.do(ctx, http.MethodHead, u)
	if err == nil && (status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented || status == http.StatusForbidden) {
		status, loc, err = e.do(ctx, http.MethodGet, u)
	}
	return status, loc, err
}

func (e *Expander) do(ctx context.Context, method string, u *url.URL) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,*/*;q=0.8")
	resp, err := e.Client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return resp.StatusCode, resp.Header.Get("Location"), nil
	}
	return resp.StatusCode, "", nil
}

// Preparer adapts Expander to the analyzer pipeline. Expansion failures do
// not abort analysis; the last reachable URL is analysed instead.
type Preparer struct{ E *Expander }

// Prepare expands t.URL and records the redirect chain.
func (p Preparer) Prepare(ctx context.Context, t *analyzer.Target) ([]analyzer.Signal, error) {
	if t.URL == nil {
		return nil, nil
	}
	startHost := strings.ToLower(t.URL.Hostname())
	res := p.E.Expand(ctx, t.URL)
	t.RedirectChain = res.Chain
	t.URL = res.Final
	t.FinalURL = res.Final.String()

	var sig []analyzer.Signal
	if Shorteners[startHost] {
		sig = append(sig, analyzer.Signal{ID: "redirect.shortener", Severity: analyzer.SeverityLow, Weight: 10,
			Detail: "link uses the URL shortener " + startHost + ", which hides the destination"})
	}
	hops := len(res.Chain) - 1
	if hops >= 3 {
		sig = append(sig, analyzer.Signal{ID: "redirect.many_hops", Severity: analyzer.SeverityMedium, Weight: 15,
			Detail: fmt.Sprintf("link bounces through %d redirects", hops)})
	}
	if hops > 0 && !strings.EqualFold(res.Final.Hostname(), startHost) {
		sig = append(sig, analyzer.Signal{ID: "redirect.cross_domain", Severity: analyzer.SeverityInfo, Weight: 0,
			Detail: "link redirects to " + res.Final.Hostname()})
	}
	if downgraded(res.Chain) {
		sig = append(sig, analyzer.Signal{ID: "redirect.https_downgrade", Severity: analyzer.SeverityMedium, Weight: 15,
			Detail: "redirect chain moves from https to plain http"})
	}
	switch {
	case errors.Is(res.Err, ErrTooManyRedirects):
		sig = append(sig, analyzer.Signal{ID: "redirect.limit_exceeded", Severity: analyzer.SeverityMedium, Weight: 20,
			Detail: fmt.Sprintf("more than %d redirects; stopped following", p.E.MaxRedirects)})
	case res.Err != nil:
		sig = append(sig, analyzer.Signal{ID: "redirect.unreachable", Severity: analyzer.SeverityInfo, Weight: 0,
			Detail: "could not fully follow the link: " + res.Err.Error()})
	}
	return sig, nil
}

func downgraded(chain []string) bool {
	seenHTTPS := false
	for _, c := range chain {
		if strings.HasPrefix(c, "https://") {
			seenHTTPS = true
		} else if seenHTTPS && strings.HasPrefix(c, "http://") {
			return true
		}
	}
	return false
}
