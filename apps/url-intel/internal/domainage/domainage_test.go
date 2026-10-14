package domainage

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

var now = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func rdapServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/domain/") {
		case "fresh-kyc-update.example":
			w.Write([]byte(`{"events":[{"eventAction":"last changed","eventDate":"2026-09-25T00:00:00Z"},{"eventAction":"registration","eventDate":"2026-09-20T10:00:00Z"}]}`))
		case "old.example":
			w.Write([]byte(`{"events":[{"eventAction":"registration","eventDate":"1999-01-01T00:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// whoisServer answers each query from responses.
func whoisServer(t *testing.T, responses map[string]string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				q, _ := bufio.NewReader(c).ReadString('\n')
				if r, ok := responses[strings.TrimSpace(q)]; ok {
					c.Write([]byte(r))
					return
				}
				c.Write([]byte("No match\n"))
			}(c)
		}
	}()
	return ln.Addr().String()
}

func newLookup(t *testing.T) *Lookup {
	l := New(http.DefaultClient)
	l.RDAPBase = rdapServer(t).URL + "/domain/"
	registry := whoisServer(t, map[string]string{
		"whois-only.example": "% fake registry\nDomain Name: WHOIS-ONLY.EXAMPLE\nCreation Date: 2026-06-15T08:00:00Z\n",
	})
	l.WHOISServer = whoisServer(t, map[string]string{
		"whois-only.example": "domain: EXAMPLE\nrefer: " + registry + "\n",
	})
	l.Dial = (&net.Dialer{}).DialContext
	l.Now = func() time.Time { return now }
	return l
}

func TestRDAP(t *testing.T) {
	l := newLookup(t)
	reg, src, err := l.Registered(context.Background(), "fresh-kyc-update.example")
	if err != nil || src != "rdap" || reg.Day() != 20 {
		t.Fatalf("%v %s %v", reg, src, err)
	}
}

func TestWHOISFallback(t *testing.T) {
	l := newLookup(t)
	reg, src, err := l.Registered(context.Background(), "whois-only.example")
	if err != nil || src != "whois" || reg.Month() != time.June {
		t.Fatalf("%v %s %v", reg, src, err)
	}
	if _, _, err := l.Registered(context.Background(), "unknown.example"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseDateFormats(t *testing.T) {
	for _, s := range []string{"2020-01-02T03:04:05Z", "2020-01-02", "02-Jan-2020", "2020-01-02 03:04:05"} {
		if _, err := parseDate(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	text := "Domain: x.in\nCreated On: 2020-01-02T03:04:05Z\n"
	if _, ok := findCreated(text); !ok {
		t.Fatal("findCreated")
	}
}

func TestCheckSignals(t *testing.T) {
	c := &Check{L: newLookup(t)}
	f := c.Run(context.Background(), &analyzer.Target{Host: "www.fresh-kyc-update.example"})
	if len(f.Signals) != 1 || f.Signals[0].ID != "domain.very_new" {
		t.Fatalf("%+v %s", f.Signals, f.Error)
	}
	f = c.Run(context.Background(), &analyzer.Target{Host: "old.example"})
	if len(f.Signals) != 0 {
		t.Fatalf("%+v", f.Signals)
	}
}
