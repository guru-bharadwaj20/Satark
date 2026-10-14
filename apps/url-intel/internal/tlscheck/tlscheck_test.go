package tlscheck

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

// checkFor returns a Check that dials srv regardless of the requested host.
func checkFor(srv *httptest.Server, roots *x509.CertPool, now time.Time) *Check {
	addr := srv.Listener.Addr().String()
	return &Check{
		Dial: func(ctx context.Context, n, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, n, addr)
		},
		Roots: roots,
		Port:  "443",
		Now:   func() time.Time { return now },
	}
}

func ids(f analyzer.Finding) map[string]bool {
	m := map[string]bool{}
	for _, s := range f.Signals {
		m[s.ID] = true
	}
	return m
}

func TestTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	leaf := srv.Certificate()
	trusted := x509.NewCertPool()
	trusted.AddCert(leaf)

	// httptest's certificate is valid for example.com.
	u, _ := url.Parse("https://example.com/")
	tgt := &analyzer.Target{Host: "example.com", URL: u}

	f := checkFor(srv, trusted, leaf.NotBefore.Add(30*24*time.Hour)).Run(context.Background(), tgt)
	if len(f.Signals) != 0 || !f.Data.(*Data).Verified {
		t.Fatalf("trusted: %+v %+v", f.Signals, f.Data)
	}

	f = checkFor(srv, x509.NewCertPool(), time.Now()).Run(context.Background(), tgt)
	if !ids(f)["tls.untrusted"] {
		t.Fatalf("untrusted: %+v", f.Signals)
	}

	f = checkFor(srv, trusted, leaf.NotAfter.Add(time.Hour)).Run(context.Background(), tgt)
	if !ids(f)["tls.expired"] {
		t.Fatalf("expired: %+v", f.Signals)
	}

	other := &analyzer.Target{Host: "sbi-kyc.example", URL: u}
	f = checkFor(srv, trusted, time.Now()).Run(context.Background(), other)
	if !ids(f)["tls.hostname_mismatch"] {
		t.Fatalf("mismatch: %+v", f.Signals)
	}

	f = checkFor(srv, trusted, leaf.NotBefore.Add(time.Hour)).Run(context.Background(), tgt)
	if !ids(f)["tls.fresh_certificate"] {
		t.Fatalf("fresh: %+v", f.Signals)
	}
}

func TestNoTLSOnHTTPSite(t *testing.T) {
	c := &Check{
		Dial: func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("connection refused") },
		Port: "443", Now: time.Now,
	}
	u, _ := url.Parse("http://plain.example/")
	f := c.Run(context.Background(), &analyzer.Target{Host: "plain.example", URL: u})
	if !ids(f)["tls.missing"] {
		t.Fatalf("%+v", f)
	}
}
