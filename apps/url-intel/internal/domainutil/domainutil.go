// Package domainutil holds helpers shared by the domain-level checks.
package domainutil

import (
	"net/netip"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// IsIP reports whether host is a literal IP address.
func IsIP(host string) bool {
	_, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return err == nil
}

// Registrable returns the registrable domain (eTLD+1) for host, e.g.
// "netbanking.hdfcbank.com" -> "hdfcbank.com", "x.sbi.co.in" -> "sbi.co.in".
// It returns host unchanged when it cannot be determined.
func Registrable(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if IsIP(host) {
		return host
	}
	d, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return d
}

// Label returns the leftmost label of the registrable domain, e.g.
// "hdfcbank.com" -> "hdfcbank".
func Label(host string) string {
	reg := Registrable(host)
	suffix, _ := publicsuffix.PublicSuffix(reg)
	l := strings.TrimSuffix(reg, "."+suffix)
	return l
}

// IsSubdomainOf reports whether host equals parent or is below it.
func IsSubdomainOf(host, parent string) bool {
	host, parent = strings.ToLower(host), strings.ToLower(parent)
	return host == parent || strings.HasSuffix(host, "."+parent)
}
