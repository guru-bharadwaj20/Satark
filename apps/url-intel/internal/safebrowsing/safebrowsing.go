// Package safebrowsing queries the Google Safe Browsing v4 Lookup API.
//
// The Lookup API is free for non-commercial use; a commercial deployment
// must switch to the Web Risk API. The API key is read from
// URL_INTEL_SAFE_BROWSING_API_KEY and the check is disabled without one.
package safebrowsing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

// DefaultEndpoint is the v4 threatMatches:find URL.
const DefaultEndpoint = "https://safebrowsing.googleapis.com/v4/threatMatches:find"

// maxEntries is the per-request limit we impose (the API allows 500).
const maxEntries = 20

var threatTypes = []string{"MALWARE", "SOCIAL_ENGINEERING", "UNWANTED_SOFTWARE", "POTENTIALLY_HARMFUL_APPLICATION"}

// Client calls the Lookup API.
type Client struct {
	HTTP     *http.Client
	Endpoint string
	APIKey   string
	Version  string
}

// New returns a Client. An empty key yields a disabled client.
func New(httpClient *http.Client, apiKey, version string) *Client {
	return &Client{HTTP: httpClient, Endpoint: DefaultEndpoint, APIKey: apiKey, Version: version}
}

// Enabled reports whether an API key is configured.
func (c *Client) Enabled() bool { return c != nil && c.APIKey != "" }

type entry struct {
	URL string `json:"url"`
}

type request struct {
	Client struct {
		ClientID      string `json:"clientId"`
		ClientVersion string `json:"clientVersion"`
	} `json:"client"`
	ThreatInfo struct {
		ThreatTypes      []string `json:"threatTypes"`
		PlatformTypes    []string `json:"platformTypes"`
		ThreatEntryTypes []string `json:"threatEntryTypes"`
		ThreatEntries    []entry  `json:"threatEntries"`
	} `json:"threatInfo"`
}

// Match is one threat match.
type Match struct {
	ThreatType string `json:"threat_type"`
	URL        string `json:"url"`
}

type response struct {
	Matches []struct {
		ThreatType string `json:"threatType"`
		Threat     entry  `json:"threat"`
	} `json:"matches"`
}

// Lookup checks urls and returns any matches.
func (c *Client) Lookup(ctx context.Context, urls []string) ([]Match, error) {
	if len(urls) > maxEntries {
		urls = urls[:maxEntries]
	}
	var body request
	body.Client.ClientID = "satark"
	body.Client.ClientVersion = c.Version
	body.ThreatInfo.ThreatTypes = threatTypes
	body.ThreatInfo.PlatformTypes = []string{"ANY_PLATFORM"}
	body.ThreatInfo.ThreatEntryTypes = []string{"URL"}
	for _, u := range urls {
		body.ThreatInfo.ThreatEntries = append(body.ThreatInfo.ThreatEntries, entry{URL: u})
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+"?key="+url.QueryEscape(c.APIKey), bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// Never leak the API key through the error string.
		return nil, fmt.Errorf("safe browsing request failed: %s", strings.ReplaceAll(err.Error(), c.APIKey, "REDACTED"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("safe browsing status %d", resp.StatusCode)
	}
	var r response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(r.Matches))
	for _, m := range r.Matches {
		out = append(out, Match{ThreatType: m.ThreatType, URL: m.Threat.URL})
	}
	return out, nil
}

// Check implements analyzer.ThreatCheck.
type Check struct{ C *Client }

// Name implements analyzer.Check.
func (*Check) Name() string { return "safe_browsing" }

// ThreatIntel marks this check as running for allowlisted hosts too.
func (*Check) ThreatIntel() {}

// Run implements analyzer.Check.
func (c *Check) Run(ctx context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	if !c.C.Enabled() {
		return f
	}
	urls := t.RedirectChain
	if len(urls) == 0 {
		urls = []string{t.FinalURL}
	}
	matches, err := c.C.Lookup(ctx, urls)
	if err != nil {
		f.Error = err.Error()
		return f
	}
	if len(matches) == 0 {
		return f
	}
	f.Data = matches
	seen := map[string]bool{}
	for _, m := range matches {
		if seen[m.ThreatType] {
			continue
		}
		seen[m.ThreatType] = true
		f.Signals = append(f.Signals, analyzer.Signal{
			ID: "safebrowsing." + strings.ToLower(m.ThreatType), Severity: analyzer.SeverityHigh, Weight: 60,
			Detail: "Google Safe Browsing lists this link as " + describe(m.ThreatType),
		})
	}
	return f
}

func describe(tt string) string {
	switch tt {
	case "SOCIAL_ENGINEERING":
		return "phishing / deceptive"
	case "MALWARE":
		return "malware"
	case "UNWANTED_SOFTWARE":
		return "unwanted software"
	case "POTENTIALLY_HARMFUL_APPLICATION":
		return "a potentially harmful app"
	}
	return strings.ToLower(tt)
}
