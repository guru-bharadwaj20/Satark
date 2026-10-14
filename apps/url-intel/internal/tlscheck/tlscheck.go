// Package tlscheck inspects the TLS certificate a host presents on port 443.
package tlscheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainutil"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/netguard"
)

// FreshCertAge marks certificates issued very recently.
const FreshCertAge = 7 * 24 * time.Hour

// Data describes the presented leaf certificate.
type Data struct {
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	DNSNames  []string  `json:"dns_names,omitempty"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	Verified  bool      `json:"verified"`
	Problem   string    `json:"problem,omitempty"`
	Version   string    `json:"tls_version"`
}

// Check implements analyzer.Check.
type Check struct {
	Dial  func(ctx context.Context, network, addr string) (net.Conn, error)
	Roots *x509.CertPool // nil means system roots
	Port  string
	Now   func() time.Time
}

// New returns a Check that only dials public addresses.
func New() *Check {
	return &Check{
		Dial: netguard.DialContext(&net.Dialer{Timeout: 3 * time.Second}),
		Port: "443",
		Now:  time.Now,
	}
}

// Name implements analyzer.Check.
func (*Check) Name() string { return "tls" }

// Inspect performs the handshake and returns certificate details.
func (c *Check) Inspect(ctx context.Context, host string) (*Data, error) {
	raw, err := c.Dial(ctx, "tcp", net.JoinHostPort(host, c.Port))
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(dl)
	} else {
		_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
	}
	// Verification is done manually below so that we can still describe
	// invalid certificates instead of just failing the handshake.
	conn := tls.Client(raw, &tls.Config{ServerName: host, InsecureSkipVerify: true, MinVersion: tls.VersionTLS10}) //nolint:gosec // inspected, not trusted
	if err := conn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	st := conn.ConnectionState()
	if len(st.PeerCertificates) == 0 {
		return nil, errors.New("no certificate presented")
	}
	leaf := st.PeerCertificates[0]
	d := &Data{
		Subject: leaf.Subject.String(), Issuer: leaf.Issuer.String(), DNSNames: leaf.DNSNames,
		NotBefore: leaf.NotBefore.UTC(), NotAfter: leaf.NotAfter.UTC(), Version: tls.VersionName(st.Version),
	}
	inter := x509.NewCertPool()
	for _, ic := range st.PeerCertificates[1:] {
		inter.AddCert(ic)
	}
	_, verr := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: c.Roots, Intermediates: inter, CurrentTime: c.Now()})
	d.Verified = verr == nil
	if verr != nil {
		d.Problem = classify(verr)
	}
	return d, nil
}

func classify(err error) string {
	var inv x509.CertificateInvalidError
	var host x509.HostnameError
	var unk x509.UnknownAuthorityError
	switch {
	case errors.As(err, &inv) && inv.Reason == x509.Expired:
		return "expired"
	case errors.As(err, &host):
		return "hostname_mismatch"
	case errors.As(err, &unk):
		return "untrusted_issuer"
	default:
		return "invalid"
	}
}

// Run implements analyzer.Check.
func (c *Check) Run(ctx context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	if domainutil.IsIP(t.Host) {
		return f
	}
	d, err := c.Inspect(ctx, t.Host)
	if err != nil {
		if t.URL != nil && t.URL.Scheme == "http" {
			f.Signals = append(f.Signals, analyzer.Signal{ID: "tls.missing", Severity: analyzer.SeverityLow, Weight: 8,
				Detail: "site does not offer HTTPS, so anything typed into it is sent unencrypted"})
		}
		f.Error = err.Error()
		return f
	}
	f.Data = d
	switch d.Problem {
	case "":
	case "expired":
		f.Signals = append(f.Signals, analyzer.Signal{ID: "tls.expired", Severity: analyzer.SeverityMedium, Weight: 15,
			Detail: "certificate expired on " + d.NotAfter.Format("2006-01-02")})
	case "hostname_mismatch":
		f.Signals = append(f.Signals, analyzer.Signal{ID: "tls.hostname_mismatch", Severity: analyzer.SeverityMedium, Weight: 15,
			Detail: "certificate is for " + strings.Join(d.DNSNames, ", ") + ", not " + t.Host})
	case "untrusted_issuer":
		sev := "certificate is not issued by a trusted authority"
		if d.Subject == d.Issuer {
			sev = "certificate is self-signed"
		}
		f.Signals = append(f.Signals, analyzer.Signal{ID: "tls.untrusted", Severity: analyzer.SeverityMedium, Weight: 20, Detail: sev})
	default:
		f.Signals = append(f.Signals, analyzer.Signal{ID: "tls.invalid", Severity: analyzer.SeverityMedium, Weight: 15,
			Detail: "certificate failed validation"})
	}
	if age := c.Now().Sub(d.NotBefore); d.Verified && age >= 0 && age < FreshCertAge {
		f.Signals = append(f.Signals, analyzer.Signal{ID: "tls.fresh_certificate", Severity: analyzer.SeverityLow, Weight: 5,
			Detail: fmt.Sprintf("certificate was issued %d hours ago", int(age.Hours()))})
	}
	return f
}
