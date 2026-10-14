// Package netguard decides which network destinations url-intel may contact.
// Untrusted URLs must never make the service reach internal addresses (SSRF).
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "64:ff9b:1::/48", "100::/64", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8",
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// IsPublic reports whether addr is a globally routable unicast address.
func IsPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// ErrBlocked is returned when a connection to a non-public address is refused.
type ErrBlocked struct{ Addr string }

func (e ErrBlocked) Error() string {
	return fmt.Sprintf("destination %s is not a public address", e.Addr)
}

// Control is a net.Dialer Control hook that refuses non-public addresses.
// It runs after DNS resolution, so it also defeats DNS rebinding.
func Control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if !IsPublic(ip) {
		return ErrBlocked{Addr: host}
	}
	return nil
}

// NewHTTPClient returns an HTTP client for outbound lookups (RDAP, CT,
// Safe Browsing) that only connects to public addresses.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil, // a proxy would bypass the address guard
			DialContext:           DialContext(&net.Dialer{Timeout: 3 * time.Second}),
			TLSHandshakeTimeout:   3 * time.Second,
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       60 * time.Second,
		},
	}
}

// DialContext returns a dial function that only connects to public addresses.
func DialContext(d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	cp := *d
	cp.Control = Control
	return cp.DialContext
}
