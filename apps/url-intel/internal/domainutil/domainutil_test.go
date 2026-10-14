package domainutil

import "testing"

func TestRegistrable(t *testing.T) {
	for in, want := range map[string]string{
		"netbanking.hdfcbank.com": "hdfcbank.com",
		"x.y.sbi.co.in":           "sbi.co.in",
		"uidai.gov.in":            "uidai.gov.in",
		"8.8.8.8":                 "8.8.8.8",
		"Example.COM.":            "example.com",
	} {
		if got := Registrable(in); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
	if Label("login.sbi.co.in") != "sbi" {
		t.Fatal(Label("login.sbi.co.in"))
	}
	if !IsSubdomainOf("a.b.gov.in", "gov.in") || IsSubdomainOf("evilgov.in", "gov.in") {
		t.Fatal("IsSubdomainOf")
	}
}
