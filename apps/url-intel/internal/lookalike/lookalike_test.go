package lookalike

import "testing"

func TestDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		d    int
	}{
		{"hdfcbank", "hdfcbank", 0}, {"hdfcbnak", "hdfcbank", 1}, {"hdfcbak", "hdfcbank", 1},
		{"icicibank", "icic1bank", 1}, {"kitten", "sitting", 3}, {"", "abc", 3},
	} {
		if got := Distance(c.a, c.b); got != c.d {
			t.Errorf("%s/%s: got %d want %d", c.a, c.b, got, c.d)
		}
	}
}

func TestFind(t *testing.T) {
	d := Default()
	cases := map[string]string{
		"hdfcbnak.com":                   "typosquat",
		"icicibamk.in":                   "typosquat",
		"paytrn.com":                     "typosquat", // rn -> m skeleton
		"flipkart-sale.example":          "brand_in_domain",
		"sbi-kyc-update.example":         "brand_in_domain",
		"hdfcbank.co":                    "brand_in_domain",
		"onlinesbi.secure-login.example": "brand_in_subdomain",
		"amaz0n.in":                      "typosquat",
	}
	for host, kind := range cases {
		m := d.Find(host)
		if m == nil || m.Kind != kind {
			t.Errorf("%s: got %+v want %s", host, m, kind)
		}
	}
	for _, host := range []string{
		"netbanking.hdfcbank.com", "www.onlinesbi.sbi", "www.irctc.co.in", "example.com",
		"insbiology.example", "wikipedia.org", "8.8.8.8",
	} {
		if m := d.Find(host); m != nil {
			t.Errorf("%s: false positive %+v", host, m)
		}
	}
}
