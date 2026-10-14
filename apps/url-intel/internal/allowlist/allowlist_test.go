package allowlist

import "testing"

func TestMatch(t *testing.T) {
	l := Default("hdfcbank.com", "onlinesbi.sbi")
	for host, want := range map[string]bool{
		"uidai.gov.in":            true,
		"www.incometax.gov.in":    true,
		"gov.in":                  false, // the bare zone is not an organisation
		"evilgov.in":              false,
		"gov.in.evil.example":     false,
		"sbi.bank.in":             true,
		"netbanking.hdfcbank.com": true,
		"hdfcbank.com.evil.in":    false,
		"www.onlinesbi.sbi":       true,
		"rbi.org.in":              true,
		"203.0.113.1":             false,
	} {
		if _, got := l.Match(host); got != want {
			t.Errorf("%s: got %v want %v", host, got, want)
		}
	}
}
