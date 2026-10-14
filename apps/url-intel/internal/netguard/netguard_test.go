package netguard

import (
	"net/netip"
	"testing"
)

func TestIsPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.0.0.1": false, "192.168.1.1": false, "169.254.169.254": false,
		"::1": false, "fd00::1": false, "::ffff:127.0.0.1": false, "100.64.0.1": false,
	} {
		if got := IsPublic(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s: got %v want %v", addr, got, want)
		}
	}
}

func TestControl(t *testing.T) {
	if err := Control("tcp", "10.0.0.1:80", nil); err == nil {
		t.Fatal("expected private address to be blocked")
	}
	if err := Control("tcp", "1.1.1.1:443", nil); err != nil {
		t.Fatalf("public address blocked: %v", err)
	}
}
