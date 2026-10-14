package homoglyph

import (
	"context"
	"testing"

	"golang.org/x/net/idna"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/lookalike"
)

func run(t *testing.T, unicodeHost string) analyzer.Finding {
	t.Helper()
	ascii, err := idna.ToASCII(unicodeHost)
	if err != nil {
		t.Fatal(err)
	}
	c := &Check{Brands: lookalike.Default()}
	return c.Run(context.Background(), &analyzer.Target{Host: ascii})
}

func signal(f analyzer.Finding) string {
	if len(f.Signals) == 0 {
		return ""
	}
	return f.Signals[0].ID
}

func TestHomoglyph(t *testing.T) {
	cases := map[string]string{
		"hdfcbаnk.com":   "homoglyph.brand",        // Cyrillic a
		"рaytm.com":      "homoglyph.brand",        // Cyrillic er
		"gооgle.com":     "homoglyph.brand",        // Cyrillic o o
		"exаmple.com":    "homoglyph.mixed_script", // no brand, but mixed
		"bücher.example": "homoglyph.punycode",
		"भारत.example":   "homoglyph.punycode",
		"example.com":    "",
	}
	for host, want := range cases {
		if got := signal(run(t, host)); got != want {
			t.Errorf("%s: got %q want %q", host, got, want)
		}
	}
}

func TestAnalyzeScripts(t *testing.T) {
	ascii, _ := idna.ToASCII("sбi.example")
	r := Analyze(ascii, nil)
	if !r.IDN || !r.MixedScript || len(r.Scripts) != 2 {
		t.Fatalf("%+v", r)
	}
}
