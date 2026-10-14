package dnscheck

import (
	"context"
	"net"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

type fakeResolver struct {
	ips map[string][]net.IPAddr
	ns  map[string][]*net.NS
	mx  map[string][]*net.MX
}

func nf(name string) error { return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true} }

func (f fakeResolver) LookupIPAddr(_ context.Context, h string) ([]net.IPAddr, error) {
	if v, ok := f.ips[h]; ok {
		return v, nil
	}
	return nil, nf(h)
}
func (f fakeResolver) LookupNS(_ context.Context, h string) ([]*net.NS, error) {
	if v, ok := f.ns[h]; ok {
		return v, nil
	}
	return nil, nf(h)
}
func (f fakeResolver) LookupMX(_ context.Context, h string) ([]*net.MX, error) {
	if v, ok := f.mx[h]; ok {
		return v, nil
	}
	return nil, nf(h)
}

func ids(f analyzer.Finding) map[string]bool {
	m := map[string]bool{}
	for _, s := range f.Signals {
		m[s.ID] = true
	}
	return m
}

func TestDNS(t *testing.T) {
	r := fakeResolver{
		ips: map[string][]net.IPAddr{
			"www.good.example": {{IP: net.ParseIP("93.184.216.34")}},
			"lan.bad.example":  {{IP: net.ParseIP("10.0.0.5")}},
		},
		ns: map[string][]*net.NS{"good.example": {{Host: "NS1.Good.Example."}}},
		mx: map[string][]*net.MX{"good.example": {{Host: "mx.good.example.", Pref: 10}}},
	}
	c := &Check{R: r}

	f := c.Run(context.Background(), &analyzer.Target{Host: "www.good.example"})
	if len(f.Signals) != 0 {
		t.Fatalf("good: unexpected %+v", f.Signals)
	}
	if d := f.Data.(*Data); d.NS[0] != "ns1.good.example" || d.MX[0] != "mx.good.example" {
		t.Fatalf("data %+v", d)
	}

	f = c.Run(context.Background(), &analyzer.Target{Host: "gone.example"})
	if !ids(f)["dns.nxdomain"] {
		t.Fatalf("nx: %+v", f.Signals)
	}

	f = c.Run(context.Background(), &analyzer.Target{Host: "lan.bad.example"})
	got := ids(f)
	if !got["dns.private_address"] || !got["dns.no_mx"] || !got["dns.no_ns"] {
		t.Fatalf("bad: %+v", f.Signals)
	}

	if f := c.Run(context.Background(), &analyzer.Target{Host: "1.2.3.4"}); f.Data != nil {
		t.Fatal("ip host should be skipped")
	}
}
