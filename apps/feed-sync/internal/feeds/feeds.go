// Package feeds downloads and parses public phishing and malware URL feeds.
package feeds

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Entry is one malicious URL from a feed, in canonical form.
type Entry struct {
	URL  string
	Host string
}

// Feed describes a source.
type Feed struct {
	Name string
	URL  string
	// Parse extracts entries; defaults to ParseLines.
	Parse func(io.Reader) ([]Entry, error)
}

// Defaults are the feeds synced by the CronJob.
var Defaults = []Feed{
	// Currently-online malware distribution URLs, updated every 5 minutes.
	{Name: "urlhaus", URL: "https://urlhaus.abuse.ch/downloads/text_online/"},
	// OpenPhish community feed, updated every 12 hours.
	{Name: "openphish", URL: "https://openphish.com/feed.txt"},
}

// MaxFeedBytes bounds a single download.
const MaxFeedBytes = 64 << 20

// MinEntries guards against storing an empty or truncated download over a
// good set: feeds this small are treated as a failed sync.
const MinEntries = 10

// ErrTooFew is returned when a feed parses to fewer than MinEntries.
var ErrTooFew = errors.New("feed returned too few entries")

// Fetcher downloads feeds.
type Fetcher struct {
	HTTP      *http.Client
	Retries   int
	Backoff   time.Duration
	UserAgent string
}

// NewFetcher returns a Fetcher with sane defaults.
func NewFetcher(timeout time.Duration) *Fetcher {
	return &Fetcher{
		HTTP:      &http.Client{Timeout: timeout},
		Retries:   3,
		Backoff:   2 * time.Second,
		UserAgent: "satark-feed-sync/1.0 (+https://github.com/guru-bharadwaj20/satark)",
	}
}

// Fetch downloads and parses f, retrying transient failures.
func (fe *Fetcher) Fetch(ctx context.Context, f Feed) ([]Entry, error) {
	var lastErr error
	for attempt := 0; attempt <= fe.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(fe.Backoff * time.Duration(1<<(attempt-1))):
			}
		}
		entries, err := fe.fetchOnce(ctx, f)
		if err == nil {
			return entries, nil
		}
		lastErr = err
		var perm permanentError
		if errors.As(err, &perm) {
			break
		}
	}
	return nil, fmt.Errorf("%s: %w", f.Name, lastErr)
}

type permanentError struct{ error }

func (fe *Fetcher) fetchOnce(ctx context.Context, f Feed) ([]Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, permanentError{err}
	}
	req.Header.Set("User-Agent", fe.UserAgent)
	resp, err := fe.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("status %d", resp.StatusCode)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, permanentError{err}
		}
		return nil, err
	}
	parse := f.Parse
	if parse == nil {
		parse = ParseLines
	}
	entries, err := parse(io.LimitReader(resp.Body, MaxFeedBytes))
	if err != nil {
		return nil, err
	}
	if len(entries) < MinEntries {
		return nil, fmt.Errorf("%w: %d", ErrTooFew, len(entries))
	}
	return entries, nil
}

// ParseLines parses one URL per line, skipping blanks and '#' comments.
// Invalid and duplicate URLs are dropped.
func ParseLines(r io.Reader) ([]Entry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	seen := map[string]bool{}
	var out []Entry
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e, ok := Canonical(line)
		if !ok || seen[e.URL] {
			continue
		}
		seen[e.URL] = true
		out = append(out, e)
	}
	return out, sc.Err()
}

// Canonical normalises a feed URL the same way url-intel normalises user
// input (lower-case scheme and host, no trailing dot, no default port, no
// fragment, "/" for an empty path) so that set lookups match exactly.
func Canonical(raw string) (Entry, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return Entry{}, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return Entry{}, false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return Entry{}, false
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	h := host
	if strings.Contains(h, ":") {
		h = "[" + h + "]"
	}
	if port != "" {
		h += ":" + port
	}
	u.Host = h
	u.User = nil
	u.Fragment, u.RawFragment = "", ""
	if u.Path == "" {
		u.Path = "/"
	}
	return Entry{URL: u.String(), Host: host}, true
}
