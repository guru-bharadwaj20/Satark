// Package domainage finds when a domain was registered, using RDAP first
// and falling back to WHOIS (port 43) for registries without RDAP.
// Scam domains are typically days or weeks old.
package domainage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainutil"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/netguard"
)

// DefaultRDAPBase uses the rdap.org bootstrap redirector.
const DefaultRDAPBase = "https://rdap.org/domain/"

// DefaultWHOISServer is asked first to discover the authoritative server.
const DefaultWHOISServer = "whois.iana.org:43"

// ErrNotFound means neither source returned a registration date.
var ErrNotFound = errors.New("registration date not found")

// Thresholds for signals.
const (
	VeryNewAge = 30 * 24 * time.Hour
	NewAge     = 180 * 24 * time.Hour
)

// Lookup finds registration dates.
type Lookup struct {
	HTTP        *http.Client
	RDAPBase    string
	WHOISServer string
	Dial        func(ctx context.Context, network, addr string) (net.Conn, error)
	Now         func() time.Time
}

// New returns a Lookup with production defaults.
func New(client *http.Client) *Lookup {
	dial := netguard.DialContext(&net.Dialer{Timeout: 3 * time.Second})
	return &Lookup{HTTP: client, RDAPBase: DefaultRDAPBase, WHOISServer: DefaultWHOISServer, Dial: dial, Now: time.Now}
}

// Data is attached to the finding.
type Data struct {
	Domain     string    `json:"domain"`
	Registered time.Time `json:"registered"`
	AgeDays    int       `json:"age_days"`
	Source     string    `json:"source"`
}

// Registered returns the registration time of domain and the source used.
func (l *Lookup) Registered(ctx context.Context, domain string) (time.Time, string, error) {
	t, rdapErr := l.rdap(ctx, domain)
	if rdapErr == nil {
		return t, "rdap", nil
	}
	t, whoisErr := l.whois(ctx, domain)
	if whoisErr == nil {
		return t, "whois", nil
	}
	return time.Time{}, "", errors.Join(fmt.Errorf("rdap: %w", rdapErr), fmt.Errorf("whois: %w", whoisErr))
}

type rdapDomain struct {
	Events []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
}

func (l *Lookup) rdap(ctx context.Context, domain string) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.RDAPBase+domain, nil)
	if err != nil {
		return time.Time{}, err
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var d rdapDomain
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return time.Time{}, err
	}
	for _, e := range d.Events {
		if strings.EqualFold(e.Action, "registration") {
			return parseDate(e.Date)
		}
	}
	return time.Time{}, ErrNotFound
}

var (
	referRe   = regexp.MustCompile(`(?im)^\s*(?:refer|whois):\s*(\S+)`)
	createdRe = regexp.MustCompile(`(?im)^\s*(?:creation date|created(?: on)?|registered(?: on)?|registration (?:date|time)|domain registration date|created date)\s*:\s*(.+?)\s*$`)
)

func (l *Lookup) whois(ctx context.Context, domain string) (time.Time, error) {
	resp, err := l.query(ctx, l.WHOISServer, domain)
	if err != nil {
		return time.Time{}, err
	}
	if t, ok := findCreated(resp); ok {
		return t, nil
	}
	m := referRe.FindStringSubmatch(resp)
	if m == nil {
		return time.Time{}, ErrNotFound
	}
	server := m[1]
	if !strings.Contains(server, ":") {
		server += ":43"
	}
	resp, err = l.query(ctx, server, domain)
	if err != nil {
		return time.Time{}, err
	}
	if t, ok := findCreated(resp); ok {
		return t, nil
	}
	return time.Time{}, ErrNotFound
}

func (l *Lookup) query(ctx context.Context, server, domain string) (string, error) {
	conn, err := l.Dial(ctx, "tcp", server)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	if _, err := fmt.Fprintf(conn, "%s\r\n", domain); err != nil {
		return "", err
	}
	var sb strings.Builder
	sc := bufio.NewScanner(io.LimitReader(conn, 256<<10))
	for sc.Scan() {
		sb.WriteString(sc.Text())
		sb.WriteByte('\n')
	}
	return sb.String(), sc.Err()
}

func findCreated(resp string) (time.Time, bool) {
	for _, m := range createdRe.FindAllStringSubmatch(resp, -1) {
		if t, err := parseDate(m[1]); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

var layouts = []string{
	time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05", "2006-01-02 15:04:05",
	"2006-01-02 15:04:05 MST", "2006-01-02", "02-Jan-2006", "2006.01.02", "02/01/2006", "2006/01/02",
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q", s)
}

// Check implements analyzer.Check.
type Check struct{ L *Lookup }

// Name implements analyzer.Check.
func (*Check) Name() string { return "domain_age" }

// Run implements analyzer.Check.
func (c *Check) Run(ctx context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	if domainutil.IsIP(t.Host) {
		return f
	}
	domain := domainutil.Registrable(t.Host)
	reg, src, err := c.L.Registered(ctx, domain)
	if err != nil {
		f.Error = err.Error()
		return f
	}
	age := c.L.Now().Sub(reg)
	f.Data = Data{Domain: domain, Registered: reg, AgeDays: int(age.Hours() / 24), Source: src}
	switch {
	case age < VeryNewAge:
		f.Signals = append(f.Signals, analyzer.Signal{ID: "domain.very_new", Severity: analyzer.SeverityHigh, Weight: 30,
			Detail: fmt.Sprintf("%s was registered only %d days ago", domain, int(age.Hours()/24))})
	case age < NewAge:
		f.Signals = append(f.Signals, analyzer.Signal{ID: "domain.new", Severity: analyzer.SeverityMedium, Weight: 12,
			Detail: fmt.Sprintf("%s was registered %d days ago", domain, int(age.Hours()/24))})
	}
	return f
}
