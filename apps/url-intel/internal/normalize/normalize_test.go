package normalize

import (
	"errors"
	"strings"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
		signal   string
	}{
		{"example.com", "http://example.com/", ""},
		{"  HTTPS://Example.COM:443/Path?q=1#frag ", "https://example.com/Path?q=1", ""},
		{"hxxp://evil-example[.]com/login", "http://evil-example.com/login", ""},
		{"<https://example.com/a>.", "https://example.com/a", ""},
		{"http://sbi.co.in@evil.example/x", "http://evil.example/x", "url.userinfo"},
		{"example.com:8443/x", "http://example.com:8443/x", "url.nonstandard_port"},
		{"http://bücher.example/", "http://xn--bcher-kva.example/", ""},
		{"http://8.8.8.8/a", "http://8.8.8.8/a", ""},
		{"http://example.com./", "http://example.com/", ""},
	}
	for _, c := range cases {
		res, err := Normalize(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got := res.URL.String(); got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
		if c.signal != "" && (len(res.Signals) == 0 || res.Signals[0].ID != c.signal) {
			t.Errorf("%q: expected signal %s, got %+v", c.in, c.signal, res.Signals)
		}
	}
}

func TestNormalizeRejects(t *testing.T) {
	cases := map[string]error{
		"":                        ErrEmpty,
		"ftp://example.com":       ErrScheme,
		"javascript:alert(1)":     ErrScheme,
		"http://127.0.0.1/":       ErrPrivateHost,
		"http://10.1.2.3/":        ErrPrivateHost,
		"http://[::1]/":           ErrPrivateHost,
		"http://169.254.169.254/": ErrPrivateHost,
		"http://localhost:8080/":  ErrPrivateHost,
		"http://nodots/":          ErrBadHost,
		"http://exa\x00mple.com":  ErrControlChars,
		"http://e.com/" + strings.Repeat("a", MaxLength): ErrTooLong,
	}
	for in, want := range cases {
		_, err := Normalize(in)
		if !errors.Is(err, want) {
			t.Errorf("%q: got %v want %v", in, err, want)
		}
		if err != nil && !errors.Is(err, analyzer.ErrInvalidInput) {
			t.Errorf("%q: error does not wrap ErrInvalidInput", in)
		}
	}
}
