// Package ctlog finds when a domain first appeared in Certificate
// Transparency logs, using the crt.sh JSON API. Phishing kits usually get a
// certificate minutes before the campaign starts, so a very recent first
// sighting is a strong signal even when WHOIS data is hidden.
package ctlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainutil"
)

// DefaultBase is the crt.sh endpoint.
const DefaultBase = "https://crt.sh/"

// RecentWindow is how new a first sighting must be to raise a signal.
const RecentWindow = 14 * 24 * time.Hour

// maxResponse bounds how much of a crt.sh answer is read.
const maxResponse = 8 << 20

// ErrNoEntries means the domain has never been logged.
var ErrNoEntries = errors.New("no certificate transparency entries")

type entry struct {
	EntryTimestamp string `json:"entry_timestamp"`
	NotBefore      string `json:"not_before"`
}

// Data is attached to the finding.
type Data struct {
	Domain    string    `json:"domain"`
	FirstSeen time.Time `json:"first_seen"`
	Entries   int       `json:"entries"`
}

// Check implements analyzer.Check.
type Check struct {
	HTTP *http.Client
	Base string
	Now  func() time.Time
}

// New returns a Check against crt.sh.
func New(client *http.Client) *Check {
	return &Check{HTTP: client, Base: DefaultBase, Now: time.Now}
}

// Name implements analyzer.Check.
func (*Check) Name() string { return "ct_first_seen" }

// FirstSeen returns the earliest CT log entry for domain.
func (c *Check) FirstSeen(ctx context.Context, domain string) (time.Time, int, error) {
	q := url.Values{"q": {domain}, "output": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"?"+q.Encode(), nil)
	if err != nil {
		return time.Time{}, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, 0, fmt.Errorf("crt.sh status %d", resp.StatusCode)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponse))
	if _, err := dec.Token(); err != nil { // opening '['
		return time.Time{}, 0, err
	}
	var first time.Time
	n := 0
	for dec.More() {
		var e entry
		if err := dec.Decode(&e); err != nil {
			return time.Time{}, 0, err
		}
		ts := e.EntryTimestamp
		if ts == "" {
			ts = e.NotBefore
		}
		t, err := parse(ts)
		if err != nil {
			continue
		}
		n++
		if first.IsZero() || t.Before(first) {
			first = t
		}
	}
	if n == 0 {
		return time.Time{}, 0, ErrNoEntries
	}
	return first, n, nil
}

func parse(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, l := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", time.RFC3339Nano} {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("bad timestamp %q", s)
}

// Run implements analyzer.Check.
func (c *Check) Run(ctx context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	if domainutil.IsIP(t.Host) {
		return f
	}
	domain := domainutil.Registrable(t.Host)
	first, n, err := c.FirstSeen(ctx, domain)
	if errors.Is(err, ErrNoEntries) {
		f.Data = Data{Domain: domain}
		if t.URL != nil && t.URL.Scheme == "https" {
			f.Signals = append(f.Signals, analyzer.Signal{ID: "ct.not_logged", Severity: analyzer.SeverityLow, Weight: 5,
				Detail: "no public certificate history for " + domain})
		}
		return f
	}
	if err != nil {
		f.Error = err.Error()
		return f
	}
	f.Data = Data{Domain: domain, FirstSeen: first, Entries: n}
	if age := c.Now().Sub(first); age < RecentWindow {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "ct.first_seen_recent", Severity: analyzer.SeverityMedium, Weight: 15,
			Detail: fmt.Sprintf("first certificate for %s appeared %d days ago", domain, int(age.Hours()/24))})
	}
	return f
}
