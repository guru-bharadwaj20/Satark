// Package dnscheck resolves A/AAAA, NS and MX records for the target host.
package dnscheck

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainutil"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/netguard"
)

// Resolver is the subset of *net.Resolver used here.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
}

// Data is the DNS evidence attached to the finding.
type Data struct {
	Addresses []string `json:"addresses,omitempty"`
	NS        []string `json:"ns,omitempty"`
	MX        []string `json:"mx,omitempty"`
	NXDomain  bool     `json:"nxdomain,omitempty"`
}

// Check implements analyzer.Check.
type Check struct{ R Resolver }

// New returns a Check using the system resolver.
func New() *Check { return &Check{R: net.DefaultResolver} }

// Name implements analyzer.Check.
func (*Check) Name() string { return "dns" }

// Run implements analyzer.Check.
func (c *Check) Run(ctx context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	if domainutil.IsIP(t.Host) {
		return f
	}
	d := &Data{}
	f.Data = d

	ips, err := c.R.LookupIPAddr(ctx, t.Host)
	if isNotFound(err) {
		d.NXDomain = true
		f.Signals = append(f.Signals, analyzer.Signal{ID: "dns.nxdomain", Severity: analyzer.SeverityMedium, Weight: 20,
			Detail: "domain does not resolve; it may have been taken down or not set up yet"})
		return f
	} else if err != nil {
		f.Error = "address lookup: " + err.Error()
	}
	private := false
	for _, ip := range ips {
		d.Addresses = append(d.Addresses, ip.IP.String())
		if a, ok := netip.AddrFromSlice(ip.IP); ok && !netguard.IsPublic(a) {
			private = true
		}
	}
	if private {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "dns.private_address", Severity: analyzer.SeverityMedium, Weight: 15,
			Detail: "domain points at a private or reserved address"})
	}

	zone := domainutil.Registrable(t.Host)
	if ns, err := c.R.LookupNS(ctx, zone); err == nil {
		for _, n := range ns {
			d.NS = append(d.NS, strings.TrimSuffix(strings.ToLower(n.Host), "."))
		}
	} else if !isNotFound(err) && f.Error == "" {
		f.Error = "ns lookup: " + err.Error()
	}
	if mx, err := c.R.LookupMX(ctx, zone); err == nil {
		for _, m := range mx {
			if h := strings.TrimSuffix(strings.ToLower(m.Host), "."); h != "" {
				d.MX = append(d.MX, h)
			}
		}
	}
	if len(d.NS) == 0 && f.Error == "" {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "dns.no_ns", Severity: analyzer.SeverityLow, Weight: 5,
			Detail: "no name servers found for " + zone})
	}
	if len(d.MX) == 0 {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "dns.no_mx", Severity: analyzer.SeverityInfo, Weight: 2,
			Detail: zone + " has no mail servers, which is unusual for a genuine business"})
	}
	return f
}

func isNotFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}
